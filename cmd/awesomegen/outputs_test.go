package main

import (
	"fmt"
	"os"
	"path/filepath"
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

// TestSwapCaseFlipsTheCaseOfTheLetters makes sure that the probe uses a
// name that is different from the original one.
func TestSwapCaseFlipsTheCaseOfTheLetters(t *testing.T) {
	require.Equal(t, "CONFIG.YAML", swapCase("config.yaml"))
	require.Equal(t, "config.yaml", swapCase("CONFIG.YAML"))
	// A name without letters cannot be told apart from its variant.
	require.Equal(t, "123", swapCase("123"))
}
