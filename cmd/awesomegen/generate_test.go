package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	ghapi "github.com/google/go-github/v55/github"
	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/davidcollom/awesomegen/internal/github"
)

// fakeScraper returns a fixed list of repositories, whatever it is asked
// for.
type fakeScraper struct {
	repos []string
	err   error
}

func (s fakeScraper) ListRepos(_ context.Context, _, _ string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.repos, nil
}

// fakeGHClient returns a fixed repository metadata, whatever it is asked
// for.
type fakeGHClient struct {
	meta github.RepoMeta
	err  error
}

func (c fakeGHClient) GetRepo(_ context.Context, _ string) (github.RepoMeta, error) {
	if c.err != nil {
		return github.RepoMeta{}, c.err
	}
	return c.meta, nil
}

// testGenerator returns a generator that works offline: the scraper returns
// one repository and the GitHub client describes it.
func testGenerator(t *testing.T) *generator {
	t.Helper()

	pushed := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	return &generator{
		Scraper: fakeScraper{repos: []string{"cli/cli"}},
		GH: fakeGHClient{meta: github.RepoMeta{
			FullName:    "cli/cli",
			URL:         "https://github.com/cli/cli",
			Description: "GitHub's official command line tool",
			Stars:       30000,
			License:     "MIT",
			Topics:      []string{"cli", "git"},
			PushedAt:    pushed,
		}},
		Clock: clockwork.NewFakeClockAt(pushed.Add(24 * time.Hour)),
		Log:   logr.Discard(),
	}
}

// runGenerateWithArgs runs a generation with the given options and
// collaborators, and returns what was written to the standard output.
func runGenerateWithArgs(t *testing.T, opts *options, gen *generator) (string, error) {
	t.Helper()

	out := &strings.Builder{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetContext(context.WithValue(context.Background(), loggerKey{}, gen.Log))

	err := runGenerateWith(cmd, opts, gen)
	return out.String(), err
}

// oneListConfig is a configuration with a single list and one source.
const oneListConfig = `lists:
  - title: "Tools"
    sources:
      - user: davidcollom
        slug: tools
`

func TestGenerateDryRunPrintsTheDocumentAndWritesNothing(t *testing.T) {
	// The whole generation runs offline, with a fake scraper and a fake
	// GitHub client.
	dir := t.TempDir()
	cfgPath := writeConfigIn(t, dir, oneListConfig)

	// An existing README must survive the dry run untouched.
	existing := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(existing, []byte("original\n"), 0o600))

	opts := &options{configPath: cfgPath, dryRun: true}
	out, err := runGenerateWithArgs(t, opts, testGenerator(t))
	require.NoError(t, err)

	// The rendered document is printed, with a comment that names the file
	// it would have been written to.
	require.Contains(t, out, "<!-- awesomegen: README.md -->")
	require.Contains(t, out, "# Tools")
	require.Contains(t, out, "cli/cli")

	// The existing file is unchanged.
	data, err := os.ReadFile(existing)
	require.NoError(t, err)
	require.Equal(t, "original\n", string(data))
}

func TestGenerateDryRunCreatesNoFile(t *testing.T) {
	// A dry run that leaves a file behind is worse than no dry run at all.
	dir := t.TempDir()
	cfgPath := writeConfigIn(t, dir, oneListConfig)

	opts := &options{configPath: cfgPath, dryRun: true}
	out, err := runGenerateWithArgs(t, opts, testGenerator(t))
	require.NoError(t, err)
	require.Contains(t, out, "# Tools")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	// Only the configuration file is in the directory.
	require.Len(t, entries, 1)
	require.Equal(t, "config.yaml", entries[0].Name())
}

func TestGenerateWritesTheDocument(t *testing.T) {
	// The same run without the dry run writes the file.
	dir := t.TempDir()
	cfgPath := writeConfigIn(t, dir, oneListConfig)

	opts := &options{configPath: cfgPath}
	out, err := runGenerateWithArgs(t, opts, testGenerator(t))
	require.NoError(t, err)
	// Nothing is printed when the document is written.
	require.Empty(t, out)

	data, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "# Tools")
	require.Contains(t, string(data), "cli/cli")
}

// TestGenerateRefusesToOverwriteWithAnUnknownList makes sure that a lookup
// that failed for a reason that has nothing to do with the repository does
// not empty a curated list.
//
// The configuration carries a curated link next to the repository that
// cannot be loaded: without the guard the remaining document would still
// be renderable - with the link alone - and it would replace the README.
// The link is what makes the test protect the guard.
func TestGenerateRefusesToOverwriteWithAnUnknownList(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfigIn(t, dir, `lists:
  - title: "Tools"
    sources:
      - user: davidcollom
        slug: tools
    categories:
      - name: "Documentation"
        items:
          - type: link
            title: "The documentation"
            url: "https://example.com/docs"
`)

	existing := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(existing, []byte("curated\n"), 0o600))

	gen := testGenerator(t)
	gen.GH = fakeGHClient{err: errors.New("the API is down")}

	opts := &options{configPath: cfgPath}
	_, err := runGenerateWithArgs(t, opts, gen)
	require.Error(t, err)
	// The message says what happened and which file was left alone.
	require.ErrorContains(t, err, "refusing to overwrite")
	require.ErrorContains(t, err, existing)

	// The curated file is still there.
	data, err := os.ReadFile(existing)
	require.NoError(t, err)
	require.Equal(t, "curated\n", string(data))
}

// TestGenerateRefusesToOverwriteWhenEveryRepositoryIsGone makes sure that a
// list whose repositories are all gone is not written.
//
// The list would be rendered as a list of links alone, and writing it
// would replace the curated README with what is left of the curation. The
// message says how to continue: the repositories have to be removed from
// the configuration.
func TestGenerateRefusesToOverwriteWhenEveryRepositoryIsGone(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfigIn(t, dir, `lists:
  - title: "Tools"
    sources:
      - user: davidcollom
        slug: tools
`)

	existing := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(existing, []byte("curated\n"), 0o600))

	gen := testGenerator(t)
	gen.GH = fakeGHClient{err: &ghapi.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusNotFound},
		Message:  "Not Found",
	}}

	opts := &options{configPath: cfgPath}
	_, err := runGenerateWithArgs(t, opts, gen)
	require.Error(t, err)
	require.ErrorContains(t, err, "every repository of the list could not be loaded")
	require.ErrorContains(t, err, "remove the repositories from the configuration")

	// The curated file is still there.
	data, err := os.ReadFile(existing)
	require.NoError(t, err)
	require.Equal(t, "curated\n", string(data))
}

// TestGenerateDryRunRejectsAnUnwritableOutput makes sure that the dry run
// reports the output paths the run would refuse.
//
// A rehearsal that accepts a path the run rejects is worse than no
// rehearsal at all: the user would believe that the run works.
func TestGenerateDryRunRejectsAnUnwritableOutput(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "docs")))

	cfgPath := writeConfigIn(t, dir, `lists:
  - title: "Tools"
    output: "docs/README.md"
    sources:
      - user: davidcollom
        slug: tools
`)

	opts := &options{configPath: cfgPath, dryRun: true}
	_, err := runGenerateWithArgs(t, opts, testGenerator(t))
	require.Error(t, err)
	require.ErrorContains(t, err, "symbolic link")
}

// TestGenerateDryRunKeepsTheCommentClosed makes sure that the path of the
// output cannot end the comment of the dry run.
//
// The comment names the file the document would have been written to. A
// path that carries the characters that end a comment would put the rest
// of the path into the document.
func TestGenerateDryRunKeepsTheCommentClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		want   string
	}{
		{"a comment in the path", "README-->.md", "README- -&gt;.md"},
		{"an angle bracket in the path", "README>.md", "README&gt;.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := writeConfigIn(t, dir, `lists:
  - title: "Tools"
    output: "`+tc.output+`"
    sources:
      - user: davidcollom
        slug: tools
`)

			opts := &options{configPath: cfgPath, dryRun: true}
			out, err := runGenerateWithArgs(t, opts, testGenerator(t))
			require.NoError(t, err)

			require.Contains(t, out, "<!-- awesomegen: "+tc.want+" -->")
			// The comment is closed exactly once: the path did not
			// end it.
			require.Equal(t, 1, strings.Count(out, "-->"))
		})
	}
}
