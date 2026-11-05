package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

// TestGenerateEmptyListNamesTheList covers source-only configurations, whose
// list slug is optional, while preserving the name used by legacy lists.
func TestGenerateEmptyListNamesTheList(t *testing.T) {
	for _, tc := range []struct {
		name string
		slug string
		want string
	}{
		{"without slug", "", "Tools"},
		{"with slug", "tools", "tools"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			body := oneListConfig
			if tc.slug != "" {
				body = strings.Replace(body, "  - title:", "  - slug: "+tc.slug+"\n    title:", 1)
			}
			cfgPath := writeConfigIn(t, dir, body)
			existing := filepath.Join(dir, "README.md")
			require.NoError(t, os.WriteFile(existing, []byte("curated\n"), 0o600))

			gen := testGenerator(t)
			gen.Scraper = fakeScraper{}
			out, err := runGenerateWithArgs(t, &options{configPath: cfgPath}, gen)
			require.EqualError(t, err, "list \""+tc.want+"\" produced no entries; refusing to overwrite "+existing+" with an empty README")
			require.Empty(t, out)

			data, err := os.ReadFile(existing)
			require.NoError(t, err)
			require.Equal(t, "curated\n", string(data))
		})
	}
}

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
//
// The test needs files whose names carry those characters, and Windows
// refuses to create a file whose name holds a ">". The escaping itself is
// covered on every platform by TestCommentTextKeepsTheCommentClosed.
func TestGenerateDryRunKeepsTheCommentClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not allow a \">\" in the name of a file")
	}
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

// TestCommentTextKeepsTheCommentClosed makes sure that a text cannot end
// the comment of the dry run.
//
// A comment must not contain "--" and it ends at the first ">", so a path
// that carries them would end the comment early and put the rest of the
// path into the document.
//
// The test works on the text alone, so that it runs on every platform: a
// file whose name holds a ">" cannot be created on Windows.
func TestCommentTextKeepsTheCommentClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"a comment in the path", "README-->.md", "README- -&gt;.md"},
		{"an angle bracket in the path", "README>.md", "README&gt;.md"},
		{"a line feed in the path", "README\n.md", "README .md"},
		{"a carriage return in the path", "README\r.md", "README .md"},
		{"a plain path", "docs/README.md", "docs/README.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, commentText(tc.in))
		})
	}
}

// TestGenerateWritesTheDocumentKeepsTheFileMode makes sure that the file
// that was replaced keeps its mode.
//
// A README that only its owner may read is widened to the mode of the
// temporary file otherwise, and the content of a public repository is not
// a secret - but the mode of the file is the choice of the user who
// curated it.
func TestGenerateWritesTheDocumentKeepsTheFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the permissions of Windows do not follow the POSIX rules")
	}
	dir := t.TempDir()
	cfgPath := writeConfigIn(t, dir, oneListConfig)

	existing := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(existing, []byte("curated\n"), 0o600))

	opts := &options{configPath: cfgPath}
	_, err := runGenerateWithArgs(t, opts, testGenerator(t))
	require.NoError(t, err)

	fi, err := os.Stat(existing)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fi.Mode().Perm(),
		"the mode of the file that was replaced is kept")
}

// TestGenerateWritesANewFileWithTheModeOfTheUmask makes sure that the mode
// of a file that did not exist yet is the one the umask of the user produces.
//
// The umask only applies while a file is created: a mode that is set
// afterwards ignores it. A checkout with a restrictive umask - 077, for
// example - ends up with a README that every user of the machine may read,
// when the mode of the temporary file is widened to 0644 explicitly.
//
// The umask belongs to the whole process, so the test does not run in
// parallel with the other tests, and the previous umask is restored when the
// test is done.
func TestGenerateWritesANewFileWithTheModeOfTheUmask(t *testing.T) {
	setUmask(t, 0o077)

	dir := t.TempDir()
	cfgPath := writeConfigIn(t, dir, oneListConfig)

	opts := &options{configPath: cfgPath}
	_, err := runGenerateWithArgs(t, opts, testGenerator(t))
	require.NoError(t, err)

	fi, err := os.Stat(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	// The umask 077 leaves the file to its owner alone: neither the group
	// nor the others may read it.
	require.Equal(t, os.FileMode(0o600), fi.Mode().Perm(),
		"the umask of the user decides the mode of a new file")
}

// TestGenerateKeepsTheModeOfAReplacedFileUnderARestrictiveUmask makes sure
// that the file that is replaced keeps its own mode, whatever the umask of
// the user is.
//
// The mode of a file that already exists is the choice of the user who
// curated it, and a restrictive umask must not widen it.
func TestGenerateKeepsTheModeOfAReplacedFileUnderARestrictiveUmask(t *testing.T) {
	setUmask(t, 0o077)

	dir := t.TempDir()
	cfgPath := writeConfigIn(t, dir, oneListConfig)

	existing := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(existing, []byte("curated\n"), 0o644))
	// The umask applies to the file that the test creates as well, so the
	// mode of the file is set explicitly.
	require.NoError(t, os.Chmod(existing, 0o644))

	opts := &options{configPath: cfgPath}
	_, err := runGenerateWithArgs(t, opts, testGenerator(t))
	require.NoError(t, err)

	fi, err := os.Stat(existing)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), fi.Mode().Perm(),
		"the mode of the file that was replaced is kept")
}

// TestTempFileModeKeepsAReplacedFilePrivate makes sure that the temporary
// file of a file that is replaced stays private while it is written.
//
// The mode of the temporary file is only widened after the content was
// written, so a temporary file that is created with the mode of the umask
// exposes the content of a README that only its owner may read - and lets
// another user of the machine change it before it is renamed into place.
func TestTempFileModeKeepsAReplacedFilePrivate(t *testing.T) {
	require.Equal(t, os.FileMode(0o600), tempFileMode(true),
		"the temporary file of a file that is replaced is private")
	require.Equal(t, os.FileMode(0o666), tempFileMode(false),
		"the umask decides the mode of the temporary file of a new file")
}

// TestCreateTempFileCreatesTheFileWithTheGivenMode makes sure that the
// temporary file is created with the mode the caller asks for.
//
// The mode of the temporary file of a file that is replaced is 0600, so that
// the content of a README that only its owner may read is not readable - or
// changeable - by anybody else while it is written. The mode of the
// temporary file of a file that does not exist yet is 0666, so that the
// umask of the user decides the mode of the new file.
//
// The umask belongs to the whole process, so the test does not run in
// parallel with the other tests, and the previous umask is restored when
// the test is done.
func TestCreateTempFileCreatesTheFileWithTheGivenMode(t *testing.T) {
	setUmask(t, 0o022)

	dir := t.TempDir()

	private, err := createTempFile(dir, 0o600)
	require.NoError(t, err)
	require.NoError(t, private.Close())

	fi, err := os.Stat(private.Name())
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fi.Mode().Perm(),
		"the temporary file of a file that is replaced is private")

	public, err := createTempFile(dir, 0o666)
	require.NoError(t, err)
	require.NoError(t, public.Close())

	fi, err = os.Stat(public.Name())
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o666&^0o022), fi.Mode().Perm(),
		"the umask of the user decides the mode of the temporary file of a new file")
}

// TestCreateTempFileUsesTheNameOfTheRun makes sure that the name of the
// temporary file tells that it belongs to a run of the generator.
//
// The name carries random characters, so that two runs of the generator do
// not pick the same one.
func TestCreateTempFileUsesTheNameOfTheRun(t *testing.T) {
	dir := t.TempDir()

	f, err := createTempFile(dir, 0o600)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	require.Regexp(t, `^\.awesomegen-[0-9a-f]{16}\.tmp$`, filepath.Base(f.Name()),
		"the name of the temporary file tells that it belongs to a run")
}

// TestCreateTempFileRetriesWhenTheNameIsTaken makes sure that a name that is
// taken is not the end of the road.
//
// A leftover of another run occupies the name, and the run that collides
// with it has to pick another name instead of failing.
func TestCreateTempFileRetriesWhenTheNameIsTaken(t *testing.T) {
	dir := t.TempDir()

	// The first name is taken by a leftover of another run, the second one
	// is free.
	used := tempFileNamesFor(t, "taken", "free")

	leftover := filepath.Join(dir, ".awesomegen-taken.tmp")
	require.NoError(t, os.WriteFile(leftover, []byte("leftover\n"), 0o600))

	f, err := createTempFile(dir, 0o600)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// The file was created with the second name.
	require.Equal(t, filepath.Join(dir, ".awesomegen-free.tmp"), f.Name())
	require.FileExists(t, f.Name())
	require.Equal(t, 2, used(), "the name that was taken is retried")

	// The leftover was left alone.
	data, err := os.ReadFile(leftover)
	require.NoError(t, err)
	require.Equal(t, "leftover\n", string(data))
}

// TestCreateTempFileCreatesTheFileExclusively makes sure that two runs
// cannot end up with the same file.
//
// The file is created exclusively, so a run that picks the name of another
// run does not take its file - it would write its content into the file of
// the other run.
func TestCreateTempFileCreatesTheFileExclusively(t *testing.T) {
	dir := t.TempDir()

	// Every name is the same one.
	used := tempFileNamesFor(t, "same")

	f, err := createTempFile(dir, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString("the content of the first run\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// The second run picks the name of the first one and has to give up.
	_, err = createTempFile(dir, 0o600)
	require.Error(t, err)
	require.ErrorContains(t, err, "no free name for a temporary file")
	require.Equal(t, tempFileNames+1, used(),
		"the helper tries every name before it gives up")

	// The create is exclusive: the name is taken.
	_, err = os.OpenFile(f.Name(), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	require.Error(t, err)
	require.True(t, os.IsExist(err), "the file is created exclusively")

	// The file of the first run still has its content.
	data, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	require.Equal(t, "the content of the first run\n", string(data))
}

// TestCreateTempFileGivesUpWhenEveryNameIsTaken makes sure that the helper
// does not try forever.
//
// The names are random, so a name that is taken is rare - but a directory
// that is full of leftovers must not hang the generator.
func TestCreateTempFileGivesUpWhenEveryNameIsTaken(t *testing.T) {
	dir := t.TempDir()

	// Every name is the same one, and it is taken.
	used := tempFileNamesFor(t, "same")

	leftover := filepath.Join(dir, ".awesomegen-same.tmp")
	require.NoError(t, os.WriteFile(leftover, []byte("leftover\n"), 0o600))

	_, err := createTempFile(dir, 0o600)
	require.Error(t, err)
	require.ErrorContains(t, err, "no free name for a temporary file")
	require.ErrorContains(t, err, dir)
	require.Equal(t, tempFileNames, used(),
		"the helper gives up after tempFileNames names")
}

// tempFileNamesFor makes the generator of the name of a temporary file
// return the names of the given list, one after the other.
//
// The last name is repeated when the list is exhausted, so that a test can
// hand a name that is taken over and over again. The generator is restored
// when the test is done, and the returned function tells how many names were
// asked for.
func tempFileNamesFor(t *testing.T, names ...string) func() int {
	t.Helper()
	require.NotEmpty(t, names, "the test has to hand at least one name")

	previous := tempFileName
	t.Cleanup(func() {
		tempFileName = previous
	})

	used := 0
	tempFileName = func() (string, error) {
		name := names[min(used, len(names)-1)]
		used++
		return name, nil
	}
	return func() int { return used }
}
