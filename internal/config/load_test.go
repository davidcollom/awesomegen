package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/davidcollom/awesomegen/internal/config"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestLoadDefaults(t *testing.T) {
	path := writeConfig(t, `
version: 1
user: davidcollom
lists:
  - slug: tools
    title: Awesome Tools
`)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, 1, cfg.Version)
	require.Len(t, cfg.Lists, 1)

	l := cfg.Lists[0]
	require.Equal(t, "misc", l.TopicFallback)
	require.Equal(t, "en-GB", l.Locale)
	require.Equal(t, 24, l.StaleMonths)
	require.Equal(t, 10, l.TopTagsLimit)
	require.Equal(t, 1, l.MinTagCount)
	require.NotNil(t, l.StarsFormat)
	require.Equal(t, config.StarsFormatLocale, *l.StarsFormat)
	require.NotNil(t, l.TopicGroupingMode)
	require.Equal(t, config.TopicGroupingModeFlat, *l.TopicGroupingMode)

	// The legacy {user + slug} fallback is applied.
	require.Len(t, l.Sources, 1)
	require.Equal(t, "davidcollom", l.Sources[0].User)
	require.Equal(t, "tools", l.Sources[0].Slug)
}

func TestLoadSourcesWithoutTopLevelUser(t *testing.T) {
	// A config that only uses lists[].sources must not require the
	// meaningless top-level "user" field.
	path := writeConfig(t, `
version: 1
lists:
  - title: Awesome Tools
    sources:
      - user: davidcollom
        slug: tools
`)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Lists[0].Sources, 1)
	require.Equal(t, "davidcollom", cfg.Lists[0].Sources[0].User)
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	// A misspelled key must be reported rather than silently ignored.
	path := writeConfig(t, `
version: 1
user: davidcollom
lists:
  - slug: tools
    title: Awesome Tools
    min_star: 500
`)
	_, err := config.Load(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "min_star")
}

func TestLoadRejectsBadValues(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unsupported version",
			body: "version: 99\nuser: d\nlists:\n  - slug: s\n    title: T\n",
			want: "unsupported config version",
		},
		{
			name: "missing lists",
			body: "version: 1\nuser: d\n",
			want: "lists is required",
		},
		{
			name: "missing title",
			body: "version: 1\nuser: d\nlists:\n  - slug: s\n",
			want: "title: is required",
		},
		{
			name: "no sources and no slug",
			body: "version: 1\nuser: d\nlists:\n  - title: T\n",
			want: "either sources or {user + slug} must be set",
		},
		{
			name: "bad stars format",
			body: "version: 1\nuser: d\nlists:\n  - slug: s\n    title: T\n    stars_format: bogus\n",
			want: "stars_format: must be one of",
		},
		{
			name: "bad grouping mode",
			body: "version: 1\nuser: d\nlists:\n  - slug: s\n    title: T\n    topic_grouping_mode: bogus\n",
			want: "topic_grouping_mode: must be one of",
		},
		{
			name: "negative min stars",
			body: "version: 1\nuser: d\nlists:\n  - slug: s\n    title: T\n    min_stars: -1\n",
			want: "min_stars: must not be negative",
		},
		{
			name: "bad locale",
			body: "version: 1\nuser: d\nlists:\n  - slug: s\n    title: T\n    locale: bogus!\n",
			want: "is not a valid BCP-47 tag",
		},
		{
			name: "absolute output",
			body: "version: 1\nuser: d\nlists:\n  - slug: s\n    title: T\n    output: /etc/passwd\n",
			want: "must be a relative path",
		},
		{
			name: "output traversal",
			body: "version: 1\nuser: d\nlists:\n  - slug: s\n    title: T\n    output: ../../etc/passwd\n",
			want: `must not contain ".."`,
		},
		{
			name: "output into a hidden directory",
			body: "version: 1\nuser: d\nlists:\n  - slug: s\n    title: T\n    output: .github/workflows/pwn.yml\n",
			want: "hidden path segments",
		},
		{
			name: "output with an unexpected extension",
			body: "version: 1\nuser: d\nlists:\n  - slug: s\n    title: T\n    output: README.sh\n",
			want: "must use one of the extensions",
		},
		{
			name: "slug containing path syntax",
			body: "version: 1\nuser: d\nlists:\n  - slug: ../../outside\n    title: T\n",
			want: "slug:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Load(writeConfig(t, tt.body))
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestLoadAcceptsSafeOutput(t *testing.T) {
	for _, out := range []string{"README.md", "docs/README.md", "lists/tools.markdown", "notes.txt"} {
		path := writeConfig(t, "version: 1\nuser: d\nlists:\n  - slug: s\n    title: T\n    output: "+out+"\n")
		_, err := config.Load(path)
		require.NoError(t, err, "output %q should be accepted", out)
	}
}

func TestSeedReposPreservesConfiguredCategories(t *testing.T) {
	l := config.List{
		Categories: []*config.Category{
			{Name: "Curated picks", Items: []*config.Item{{Type: config.ItemLink, URL: "https://example.com"}}},
		},
	}
	l.SeedRepos([]string{"a/b", "c/d"})

	require.Len(t, l.Categories, 2)
	require.Equal(t, "Curated picks", l.Categories[0].Name)
	require.Len(t, l.Categories[0].Items, 1, "manually configured items must not be discarded")
	require.Equal(t, "Repositories", l.Categories[1].Name)
	require.Len(t, l.Categories[1].Items, 2)
}
