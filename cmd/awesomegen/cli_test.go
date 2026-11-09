package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRunCLIReturnsStatusAndStopsContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		arg  string
		code int
	}{
		{"success", "--version", 0},
		{"error", "--unknown-option", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newRootCommand()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs([]string{tc.arg})
			ctx, stop := context.WithCancel(context.Background())
			t.Cleanup(stop)

			require.Equal(t, tc.code, runCLI(ctx, root, stop))
			require.ErrorIs(t, ctx.Err(), context.Canceled)
		})
	}
}

// run executes the command tree with the given arguments and returns what it
// wrote to the standard output.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// writeConfig writes a configuration file into a temporary directory and
// returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()

	return writeConfigIn(t, t.TempDir(), body)
}

// writeConfigIn writes a configuration file into the given directory and
// returns its path.
func writeConfigIn(t *testing.T, dir, body string) string {
	t.Helper()

	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

const validConfig = `version: 1
user: davidcollom
lists:
  - slug: tools
    title: "Tools"
    tagline: "Tools for the trade"
    sources:
      - user: davidcollom
        slug: tools
    min_stars: 10
    stale_months: 0
    stars_format: compact
    categories:
      - name: Curated
        items:
          - repo: cli/cli
            title: "CLI"
          - url: https://example.com
            title: "Example"
`

func TestValidateAcceptsAValidConfig(t *testing.T) {
	path := writeConfig(t, validConfig)

	out, err := run(t, "validate", "--config", path)
	require.NoError(t, err)
	require.Contains(t, out, "is valid")
	require.Contains(t, out, "Tools")
	require.Contains(t, out, "davidcollom/tools")
	require.Contains(t, out, "README.md")
	// The stale check is disabled by an explicit zero, so the summary has to
	// report zero rather than the default.
	require.Contains(t, out, "0 month(s)")
	require.Contains(t, out, "compact")
}

func TestValidateRejectsAnInvalidConfig(t *testing.T) {
	path := writeConfig(t, "lists: []\n")

	_, err := run(t, "validate", "--config", path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "lists is required")
}

func TestValidateRejectsAnUnknownField(t *testing.T) {
	// A misspelled key is reported instead of being silently ignored.
	path := writeConfig(t, "lists:\n  - title: \"Tools\"\n    min_star: 10\n")

	_, err := run(t, "validate", "--config", path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "min_star")
}

func TestValidateReportsAMissingFile(t *testing.T) {
	_, err := run(t, "validate", "--config", filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err)
}

func TestSchemaPrintsValidJSON(t *testing.T) {
	out, err := run(t, "schema")
	require.NoError(t, err)
	require.NotEmpty(t, out)

	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &doc), "the schema must be valid JSON")
	require.Equal(t, "https://json-schema.org/draft-07/schema", doc["$schema"])
}

func TestSchemaMatchesTheCommittedFile(t *testing.T) {
	// The embedded schema is the file at the root of the repository, so the
	// two must not drift apart.
	committed, err := os.ReadFile(filepath.Join("..", "..", "config-schema.json"))
	require.NoError(t, err)

	out, err := run(t, "schema")
	require.NoError(t, err)
	require.JSONEq(t, string(committed), out)
}

func TestVersionPrintsTheVersion(t *testing.T) {
	out, err := run(t, "version")
	require.NoError(t, err)
	require.Contains(t, out, "awesomegen ")
	require.Contains(t, out, version)
	require.Contains(t, out, "go:")
}

func TestRootCommandRejectsArguments(t *testing.T) {
	_, err := run(t, "unexpected")
	require.Error(t, err)
}

func TestDryRunPrintsTheDocumentAndLeavesTheFileAlone(t *testing.T) {
	// The dry run has to print the rendered document and leave the
	// destination untouched: a dry run that overwrote the README would be
	// worse than no dry run at all.
	root := t.TempDir()
	out := filepath.Join(root, "README.md")
	require.NoError(t, os.WriteFile(out, []byte("original\n"), 0o600))

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	require.NoError(t, writeOutput(cmd, root, out, "rendered\n", true))

	// The document is printed, with a comment that names the file it would
	// have been written to.
	require.Contains(t, buf.String(), "<!-- awesomegen: README.md -->")
	require.Contains(t, buf.String(), "rendered\n")

	// The file on disk is untouched.
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Equal(t, "original\n", string(data))
}

func TestWriteOutputWritesTheFile(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "docs", "README.md")

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	require.NoError(t, writeOutput(cmd, root, out, "rendered\n", false))

	// Nothing is printed when the document is written.
	require.Empty(t, buf.String())

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Equal(t, "rendered\n", string(data))
}

func TestValidateRejectsASymlinkedOutputDirectory(t *testing.T) {
	// A symlinked directory would move the generated file outside of the
	// directory the configuration lives in. The offline check has to catch
	// that, not only the write.
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "docs")))

	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(
		"lists:\n  - title: \"Tools\"\n    output: \"docs/README.md\"\n    sources:\n      - user: \"davidcollom\"\n        slug: \"tools\"\n",
	), 0o600))

	_, err := run(t, "validate", "--config", path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "symbolic link")
}

func TestValidateRejectsAnImpossibleUser(t *testing.T) {
	// The scraper refuses a user that is not a valid GitHub user name, so
	// the offline check has to refuse it as well.
	path := writeConfig(t, "lists:\n  - title: \"Tools\"\n    sources:\n      - user: \"bad/name\"\n        slug: \"tools\"\n")

	_, err := run(t, "validate", "--config", path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a valid GitHub user")
}

func TestValidateRejectsADirectoryAsTheOutput(t *testing.T) {
	// A directory that carries the name of the README has an allowed
	// extension and is not a symbolic link, so it passes every other check.
	// The write would fail when the temporary file is renamed over it, so
	// the offline check has to refuse it.
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "README.md"), 0o755))

	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(
		"lists:\n  - title: \"Tools\"\n    sources:\n      - user: \"davidcollom\"\n        slug: \"tools\"\n",
	), 0o600))

	_, err := run(t, "validate", "--config", path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "is a directory")
}

func TestValidateRejectsAFileAsTheOutputDirectory(t *testing.T) {
	// A regular file cannot hold the README either: the write would fail
	// when it tries to create the directory.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs"), []byte("not a directory\n"), 0o600))

	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(
		"lists:\n  - title: \"Tools\"\n    output: \"docs/README.md\"\n    sources:\n      - user: \"davidcollom\"\n        slug: \"tools\"\n",
	), 0o600))

	_, err := run(t, "validate", "--config", path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "is not a directory")
}

// TestValidateRejectsNestedOutputs makes sure that two output paths where one
// is the directory of the other are refused.
//
// Both paths are valid on their own and neither exists yet, so every other
// check passes. The generator cannot write both, though: it would write one
// of them and then fail on the other one, because the file is in the way of
// the directory it needs.
func TestValidateRejectsNestedOutputs(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(
		"lists:\n"+
			"  - title: \"Tools\"\n    output: \"docs.md\"\n    sources:\n      - user: \"davidcollom\"\n        slug: \"tools\"\n"+
			"  - title: \"More\"\n    output: \"docs.md/README.md\"\n    sources:\n      - user: \"davidcollom\"\n        slug: \"more\"\n",
	), 0o600))

	_, err := run(t, "validate", "--config", path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "which is inside")
}

func TestWriteOutputRejectsADirectoryAsTheDestination(t *testing.T) {
	// The write refuses the directory as well, and says so.
	root := t.TempDir()
	out := filepath.Join(root, "README.md")
	require.NoError(t, os.Mkdir(out, 0o755))

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := writeOutput(cmd, root, out, "rendered\n", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "is a directory")
}

func TestGenerateRejectsAnInvalidConfig(t *testing.T) {
	path := writeConfig(t, "lists: []\n")

	_, err := run(t, "generate", "--config", path)
	require.Error(t, err)
}

// TestCommandsRejectTheFlagsTheyDoNotUse makes sure that a flag a command
// does not use is refused instead of being ignored.
//
// The flags are persistent flags of the root command, so every subcommand
// accepts them. A flag that is accepted and ignored lies to the user:
// "validate --dry-run" looks like a rehearsal, and it is not.
//
// The table covers the shapes a flag can be written in, so that a check that
// only catches one of them does not pass the suite.
func TestCommandsRejectTheFlagsTheyDoNotUse(t *testing.T) {
	path := writeConfig(t, validConfig)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"validate refuses the dry run", []string{"validate", "--config", path, "--dry-run"}, "--dry-run is not used by the validate command"},
		{"validate refuses the dry run with a value", []string{"validate", "--dry-run=true"}, "--dry-run is not used by the validate command"},
		{"validate refuses the dry run that is switched off", []string{"validate", "--dry-run=false"}, "--dry-run is not used by the validate command"},
		{"schema refuses the config file", []string{"schema", "--config", path}, "--config is not used by the schema command"},
		{"schema refuses the config file with a value", []string{"schema", "--config=" + path}, "--config is not used by the schema command"},
		{"schema refuses the short config file", []string{"schema", "-c", path}, "--config is not used by the schema command"},
		{"schema refuses the dry run", []string{"schema", "--dry-run"}, "--dry-run is not used by the schema command"},
		{"version refuses the config file", []string{"version", "--config", path}, "--config is not used by the version command"},
		{"version refuses the dry run", []string{"version", "--dry-run"}, "--dry-run is not used by the version command"},
		{"the help command refuses the dry run", []string{"help", "--dry-run"}, "--dry-run is not used by the help command"},
		{"the completion command refuses the dry run", []string{"completion", "--dry-run"}, "--dry-run is not used by the completion command"},
		{"the completion of a shell refuses the config file", []string{"completion", "bash", "--config", path}, "--config is not used by the bash command"},
		// The flags are shared, so a flag that is written before the
		// name of the subcommand belongs to the subcommand as well.
		{"a flag before the subcommand belongs to the subcommand", []string{"--dry-run", "validate"}, "--dry-run is not used by the validate command"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(t, tc.args...)
			require.Error(t, err)
			// The message names the flag and the command, so that the
			// user knows what to change.
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestCommandsAcceptTheFlagsTheyUse makes sure that the flags a command does
// use keep working.
//
// The check of the unused flags must not refuse the flags that the command
// honours.
func TestCommandsAcceptTheFlagsTheyUse(t *testing.T) {
	path := writeConfig(t, validConfig)

	out, err := run(t, "validate", "--config", path)
	require.NoError(t, err)
	require.Contains(t, out, "is valid")

	out, err = run(t, "schema")
	require.NoError(t, err)
	require.NotEmpty(t, out)

	out, err = run(t, "version")
	require.NoError(t, err)
	require.Contains(t, out, "awesomegen ")

	// The verbose flag is read by the pre-run hook of the root command,
	// so every command honours it.
	_, err = run(t, "schema", "--verbose")
	require.NoError(t, err)

	// The root command and the generate command are the two that use the
	// config file and the dry run, so they have to accept them.
	//
	// The run itself contacts GitHub, and the tests do not touch the
	// network, so the configuration is one that the loader refuses: the
	// run stops before the first request, and the error is the one of the
	// configuration rather than the refusal of a flag.
	badPath := writeConfig(t, "lists: []\n")
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"the root command accepts the config file and the dry run", []string{"--dry-run", "--config", badPath}},
		{"the generate command accepts the config file and the dry run", []string{"generate", "--dry-run", "--config", badPath}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(t, tc.args...)
			require.Error(t, err)
			// The run reached the configuration file, so the flags were
			// accepted and the config file was read.
			require.Contains(t, err.Error(), "lists is required")
			require.NotContains(t, err.Error(), "is not used by")
		})
	}
}

// TestCompletionWritesTheScriptToTheOutput makes sure that the completion
// script goes to the output writer of the command.
//
// Cobra captures the writer when the completion command is built, so the
// script used to end up in the standard output stream even when the caller
// had set another writer.
func TestCompletionWritesTheScriptToTheOutput(t *testing.T) {
	for _, tc := range []struct {
		shell string
		want  string
	}{
		{"bash", "# bash completion V2 for awesomegen"},
		{"zsh", "#compdef awesomegen"},
		{"fish", "# fish completion for awesomegen"},
		{"powershell", "# powershell completion for awesomegen"},
	} {
		t.Run(tc.shell, func(t *testing.T) {
			out, err := run(t, "completion", tc.shell)
			require.NoError(t, err)
			require.Contains(t, out, tc.want)
		})
	}
}

// TestCompletionRefusesAnArgument makes sure that the completion command
// refuses an argument.
//
// The command is runnable now, so cobra checks the arguments and reports the
// unknown one instead of printing the help.
func TestCompletionRefusesAnArgument(t *testing.T) {
	out, err := run(t, "completion", "nonsense")
	require.Error(t, err)
	require.Contains(t, err.Error(), `unknown command "nonsense" for "awesomegen completion"`)
	// The help is not printed, the argument is refused.
	require.NotContains(t, out, "Generate the autocompletion script")
}

func TestUnknownSubcommandFails(t *testing.T) {
	_, err := run(t, "nope")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "nope") || strings.Contains(err.Error(), "unknown"))
}
