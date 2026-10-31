package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/davidcollom/awesomegen/internal/config"
)

// TestResolveOutputsRefusesTwoOutputsThatDifferOnlyInCase makes sure that
// two lists cannot write to the same file.
//
// A file system that ignores the case treats "README.md" and "readme.md" as
// the same name, so the second list would overwrite the first one.
func TestResolveOutputsRefusesTwoOutputsThatDifferOnlyInCase(t *testing.T) {
	cfg := config.Config{Lists: []config.List{
		{Title: "One", Output: "README.md"},
		{Title: "Two", Output: "readme.md"},
	}}

	_, err := resolveOutputsIn(cfg, t.TempDir(), outputPolicy{caseInsensitive: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "both write to")

	// A file system that tells the names apart has two different files
	// here, so the two outputs are allowed.
	out, err := resolveOutputsIn(cfg, t.TempDir(), outputPolicy{caseInsensitive: false})
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.NotEqual(t, out[0], out[1])
}

// TestResolveOutputsRefusesAnOutputInsideAnotherOutput makes sure that a
// file and a directory that need the same name are refused.
//
// The generator would write the file and then fail on the directory,
// because the file is in the way of the directory it needs.
func TestResolveOutputsRefusesAnOutputInsideAnotherOutput(t *testing.T) {
	cfg := config.Config{Lists: []config.List{
		{Title: "File", Output: "docs.md"},
		{Title: "Directory", Output: "docs.md/README.md"},
	}}

	_, err := resolveOutputsIn(cfg, t.TempDir(), outputPolicy{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "which is inside")
}

// TestResolveOutputsRefusesAnOutputInsideAnotherOutputIgnoringTheCase makes
// sure that the directory check follows the case policy of the file system.
//
// On a file system that ignores the case, "docs.md" and "Docs.md" are the
// same name: the file is in the way of the directory the other list needs.
func TestResolveOutputsRefusesAnOutputInsideAnotherOutputIgnoringTheCase(t *testing.T) {
	cfg := config.Config{Lists: []config.List{
		{Title: "File", Output: "docs.md"},
		{Title: "Directory", Output: "Docs.md/readme.md"},
	}}

	_, err := resolveOutputsIn(cfg, t.TempDir(), outputPolicy{caseInsensitive: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "which is inside")

	// A file system that tells the names apart has two different names
	// here.
	_, err = resolveOutputsIn(cfg, t.TempDir(), outputPolicy{caseInsensitive: false})
	require.NoError(t, err)
}

// TestResolveOutputsStaysFastWithManyLists makes sure that the check of the
// output paths does not compare every path with every other one.
//
// The number of comparisons has to grow with the depth of the paths and not
// with the square of the number of lists, so that a configuration with many
// lists stays cheap to check.
func TestResolveOutputsStaysFastWithManyLists(t *testing.T) {
	const lists = 5000
	cfg := config.Config{Lists: make([]config.List, 0, lists)}
	for i := range lists {
		cfg.Lists = append(cfg.Lists, config.List{
			Title:  fmt.Sprintf("List %d", i),
			Output: fmt.Sprintf("lists/list-%d.md", i),
		})
	}

	start := time.Now()
	out, err := resolveOutputsIn(cfg, t.TempDir(), outputPolicy{})
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Len(t, out, lists)

	// The limit is generous: the check needs a few milliseconds, and it
	// needs minutes when every path is compared with every other one.
	require.Less(t, elapsed, 5*time.Second, "the output check is too slow")
}

// TestCaseInsensitiveDirFollowsTheFileSystem makes sure that the probe of
// the file system agrees with the file system itself.
func TestCaseInsensitiveDirFollowsTheFileSystem(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("x"), 0o600))

	_, err := os.Stat(filepath.Join(dir, "CONFIG.YAML"))
	require.Equal(t, err == nil, caseInsensitiveDir(dir),
		"the probe has to agree with the file system")
}

// TestCaseInsensitiveDirProbesEveryName makes sure that the probe looks at
// every name of the directory.
//
// The first name that carries letters does not have to be the one that gives
// the answer away: a name that cannot be swapped - a dangling symbolic
// link, for example - is not found on a file system that ignores the case
// either. A probe that stops at that name reports a case-sensitive file
// system, and two outputs that differ only in case are then allowed: the
// second list silently overwrites the README of the first one.
//
// The file system is played by the injected lookup, so that the test says
// the same thing on a runner whose file system tells the cases apart.
func TestCaseInsensitiveDirProbesEveryName(t *testing.T) {
	dir := t.TempDir()
	// The symbolic link dangles, so its variant cannot be resolved, and
	// its name sorts before the name of the file: it is the first name
	// that carries letters.
	require.NoError(t, os.Symlink("missing", filepath.Join(dir, "Zzz")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.md"), []byte("x"), 0o600))

	require.True(t, caseInsensitiveDirWith(dir, existsIgnoringCase(t, dir)),
		"the probe has to look at every name, not only at the first one")
}

// TestCaseInsensitiveDirSkipsNamesWithoutLetters makes sure that a name
// that carries no letters is not looked up.
//
// Its variant is the name itself, so a lookup of it says nothing about the
// file system.
func TestCaseInsensitiveDirSkipsNamesWithoutLetters(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "123"), []byte("x"), 0o600))

	var looked []string
	caseInsensitiveDirWith(dir, func(path string) bool {
		looked = append(looked, path)
		return false
	})

	require.Empty(t, looked, "a name without letters must not be looked up")
}

// TestCaseInsensitiveDirReportsAnUnreadableDirectoryAsCaseSensitive makes
// sure that a directory that cannot be read is reported as case-sensitive.
//
// It is the safe answer: two outputs that differ only in case are then
// allowed, exactly as they are on a file system that tells them apart.
func TestCaseInsensitiveDirReportsAnUnreadableDirectoryAsCaseSensitive(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	require.False(t, caseInsensitiveDir(missing))
}

// TestSwapCaseFlipsTheCaseOfTheLetters makes sure that the probe uses a
// name that is different from the original one.
func TestSwapCaseFlipsTheCaseOfTheLetters(t *testing.T) {
	require.Equal(t, "CONFIG.YAML", swapCase("config.yaml"))
	require.Equal(t, "config.yaml", swapCase("CONFIG.YAML"))
	// A name without letters cannot be told apart from its variant.
	require.Equal(t, "123", swapCase("123"))
}

// existsIgnoringCase plays a file system that ignores the case.
//
// A name is found when the directory holds an entry that carries it,
// whatever the case of its letters is. The entry has to be there, though: a
// symbolic link that dangles is not found, because the file behind it is
// missing.
func existsIgnoringCase(t *testing.T, dir string) func(string) bool {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := map[string]bool{}
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(dir, e.Name())); err != nil {
			// The link is followed, exactly as it is on a file system
			// that ignores the case.
			continue
		}
		names[strings.ToLower(e.Name())] = true
	}

	return func(path string) bool {
		return names[strings.ToLower(filepath.Base(path))]
	}
}
