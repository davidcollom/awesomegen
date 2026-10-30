package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/davidcollom/awesomegen/internal/config"
)

// configPathIn returns a configuration file inside a temporary directory,
// together with the directory that holds it.
//
// The tests use a temporary directory rather than a fixed path because
// resolveOutputs resolves the output paths against the directory of the
// configuration file with filepath.Abs. On Windows that returns a drive
// qualified path, so a fixed "/data" would not be comparable with the
// result.
func configPathIn(t *testing.T) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	return dir, filepath.Join(dir, "config.yaml")
}

func TestResolveOutputsSingleListDefaultsToReadme(t *testing.T) {
	dir, cfgPath := configPathIn(t)
	cfg := config.Config{Lists: []config.List{{Slug: "tools"}}}
	out, err := resolveOutputs(cfg, cfgPath)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, filepath.Join(dir, "README.md"), out[0])
}

func TestResolveOutputsMultipleListsGetDistinctFiles(t *testing.T) {
	dir, cfgPath := configPathIn(t)
	cfg := config.Config{Lists: []config.List{{Slug: "tools"}, {Slug: "k8s"}}}
	out, err := resolveOutputs(cfg, cfgPath)
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, filepath.Join(dir, "tools.md"), out[0])
	require.Equal(t, filepath.Join(dir, "k8s.md"), out[1])
}

func TestResolveOutputsRejectsCollisions(t *testing.T) {
	_, cfgPath := configPathIn(t)
	cfg := config.Config{Lists: []config.List{
		{Slug: "a", Output: "README.md"},
		{Slug: "b", Output: "README.md"},
	}}
	_, err := resolveOutputs(cfg, cfgPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "both write to")
}

func TestResolveOutputsResolvesRelativeToConfigDir(t *testing.T) {
	dir, cfgPath := configPathIn(t)
	cfg := config.Config{Lists: []config.List{{Slug: "tools", Output: "docs/README.md"}}}
	out, err := resolveOutputs(cfg, cfgPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "docs", "README.md"), out[0])
}

func TestResolveOutputsRequiresOutputForUnnamedLists(t *testing.T) {
	_, cfgPath := configPathIn(t)
	cfg := config.Config{Lists: []config.List{{}, {Slug: "b"}}}
	_, err := resolveOutputs(cfg, cfgPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "output is required")
}

func TestResolveOutputsRejectsUnsafeDerivedPaths(t *testing.T) {
	// A slug is user supplied and the derived file name must not be able to
	// escape the config directory or reach a hidden path.
	_, cfgPath := configPathIn(t)
	for _, slug := range []string{"../../outside", ".git", ".."} {
		cfg := config.Config{Lists: []config.List{{Slug: slug}, {Slug: "b"}}}
		_, err := resolveOutputs(cfg, cfgPath)
		require.Error(t, err, "slug %q must not produce an output path", slug)
	}
}

func TestWriteFileAtomicRejectsSymlinkedDirectory(t *testing.T) {
	// A symlinked destination directory would move the generated file
	// outside of the directory the configuration lives in.
	root := t.TempDir()
	outside := t.TempDir()

	require.NoError(t, os.Symlink(outside, filepath.Join(root, "docs")))

	err := writeFileAtomic(root, filepath.Join(root, "docs", "README.md"), []byte("hello"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "symbolic link")

	// Nothing was written through the symlink.
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestWriteFileAtomicRejectsPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "README.md")

	err := writeFileAtomic(root, outside, []byte("hello"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "outside of")

	_, statErr := os.Stat(outside)
	require.True(t, os.IsNotExist(statErr))
}

func TestWriteFileAtomicWritesInsideRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docs", "README.md")

	require.NoError(t, writeFileAtomic(root, target, []byte("hello")))

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))
}
