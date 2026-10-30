package github_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/davidcollom/awesomegen/internal/github"
)

func TestSplitOwnerRepo(t *testing.T) {
	valid := []struct{ in, owner, repo string }{
		{"davidcollom/awesomegen", "davidcollom", "awesomegen"},
		{"cli/cli", "cli", "cli"},
		{"owner/repo.with.dots", "owner", "repo.with.dots"},
		{"owner/repo_name", "owner", "repo_name"},
		{"Owner-1/Repo-2", "Owner-1", "Repo-2"},
	}
	for _, tc := range valid {
		owner, repo, err := github.SplitOwnerRepo(tc.in)
		require.NoError(t, err, tc.in)
		require.Equal(t, tc.owner, owner)
		require.Equal(t, tc.repo, repo)
	}

	invalid := []string{
		"",
		"owner",
		"owner/",
		"/repo",
		"owner/repo/extra",
		// Path traversal: go-github does not escape path segments.
		"../../user/repos",
		"owner/../user",
		"owner/repo?x=1",
		"owner/repo#frag",
		"own er/repo",
		"owner/re po",
		"owner/repo\n",
		// "." and ".." pass the character allowlist but are path segments:
		// "owner/.." resolves to the endpoint of the parent collection.
		"owner/..",
		"owner/.",
		"./repo",
		"../repo",
	}
	for _, in := range invalid {
		_, _, err := github.SplitOwnerRepo(in)
		require.Error(t, err, "expected %q to be rejected", in)
	}
}
