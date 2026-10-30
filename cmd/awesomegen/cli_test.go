package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

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

func TestGenerateRejectsAnInvalidConfig(t *testing.T) {
	path := writeConfig(t, "lists: []\n")

	_, err := run(t, "generate", "--config", path)
	require.Error(t, err)
}

func TestUnknownSubcommandFails(t *testing.T) {
	_, err := run(t, "nope")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "nope") || strings.Contains(err.Error(), "unknown"))
}
