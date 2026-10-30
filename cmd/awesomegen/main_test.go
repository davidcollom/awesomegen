package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/davidcollom/awesomegen/internal/config"
)

func TestResolveOutputsSingleListDefaultsToReadme(t *testing.T) {
	cfg := config.Config{Lists: []config.List{{Slug: "tools"}}}
	out, err := resolveOutputs(cfg, "/data/config.yaml")
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, filepath.Join("/data", "README.md"), out[0])
}

func TestResolveOutputsMultipleListsGetDistinctFiles(t *testing.T) {
	cfg := config.Config{Lists: []config.List{{Slug: "tools"}, {Slug: "k8s"}}}
	out, err := resolveOutputs(cfg, "/data/config.yaml")
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, filepath.Join("/data", "tools.md"), out[0])
	require.Equal(t, filepath.Join("/data", "k8s.md"), out[1])
}

func TestResolveOutputsRejectsCollisions(t *testing.T) {
	cfg := config.Config{Lists: []config.List{
		{Slug: "a", Output: "README.md"},
		{Slug: "b", Output: "README.md"},
	}}
	_, err := resolveOutputs(cfg, "/data/config.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "both write to")
}

func TestResolveOutputsResolvesRelativeToConfigDir(t *testing.T) {
	cfg := config.Config{Lists: []config.List{{Slug: "tools", Output: "docs/README.md"}}}
	out, err := resolveOutputs(cfg, "/data/config.yaml")
	require.NoError(t, err)
	require.Equal(t, filepath.Join("/data", "docs", "README.md"), out[0])
}

func TestResolveOutputsRequiresOutputForUnnamedLists(t *testing.T) {
	cfg := config.Config{Lists: []config.List{{}, {Slug: "b"}}}
	_, err := resolveOutputs(cfg, "/data/config.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "output is required")
}

func TestResolveOutputsRejectsUnsafeDerivedPaths(t *testing.T) {
	// A slug is user supplied and the derived file name must not be able to
	// escape the config directory or reach a hidden path.
	for _, slug := range []string{"../../outside", ".git", ".."} {
		cfg := config.Config{Lists: []config.List{{Slug: slug}, {Slug: "b"}}}
		_, err := resolveOutputs(cfg, "/data/config.yaml")
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
