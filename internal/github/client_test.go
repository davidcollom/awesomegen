package github_test

import (
	"net/http"
	"net/url"
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

// TestCheckRedirect makes sure that a redirect cannot hand the token to
// another host or send it over plain HTTP.
//
// The token is added to every request of a redirect chain, so a redirect to
// another host or to another scheme has to be refused.
func TestCheckRedirect(t *testing.T) {
	first := &http.Request{URL: &url.URL{Scheme: "https", Host: "api.github.com"}}

	redirect := func(target string) *http.Request {
		u, err := url.Parse(target)
		require.NoError(t, err)
		return &http.Request{URL: u}
	}

	// The same host over the same scheme is followed.
	require.NoError(t, github.CheckRedirect(redirect("https://api.github.com/repos/other/repo"), []*http.Request{first}))
	// A different host would hand the token over.
	require.Error(t, github.CheckRedirect(redirect("https://attacker.example/repos/other/repo"), []*http.Request{first}))
	// The same host over plain HTTP would send the token in clear text.
	require.Error(t, github.CheckRedirect(redirect("http://api.github.com/repos/other/repo"), []*http.Request{first}))
	// The first request of a chain is never a redirect.
	require.NoError(t, github.CheckRedirect(first, nil))
}
