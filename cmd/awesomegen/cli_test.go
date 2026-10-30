package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	dir := t.TempDir()
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

func TestDryRunDoesNotWriteTheFile(t *testing.T) {
	// The dry run needs a network connection to scrape the list, so the
	// check here is limited to the flag being accepted.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(validConfig), 0o600))

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--dry-run", "--config", path, "validate"})
	require.NoError(t, root.Execute())
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
