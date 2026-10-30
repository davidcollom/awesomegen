package scrape_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-logr/stdr"
	"github.com/stretchr/testify/require"

	"github.com/davidcollom/awesomegen/internal/scrape"
)

// newServer starts a test server that serves the given fixture for every
// request and returns a scraper pointed at it.
func newServer(t *testing.T, fixture string) (*scrape.HTMLScraper, *httptest.Server) {
	t.Helper()
	html, err := os.ReadFile(fixture)
	require.NoError(t, err, "reading fixture %s", fixture)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(html)
	}))
	t.Cleanup(srv.Close)

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL
	return sc, srv
}

func TestListRepos(t *testing.T) {
	sc, _ := newServer(t, "../../testdata/stars_list_page.html")

	out, err := sc.ListRepos(context.Background(), "davidcollom", "tools")
	require.NoError(t, err)
	require.NotEmpty(t, out)

	require.Contains(t, out, "davidcollom/awesomegen")
	require.Contains(t, out, "cli/cli")
	require.Contains(t, out, "golang/go")

	// Links that are not repositories must not be collected.
	require.NotContains(t, out, "topics/awesome")
	require.NotContains(t, out, "marketplace/actions")
	require.NotContains(t, out, "sponsors/davidcollom")
	require.NotContains(t, out, "contact/report-abuse")
	// Three-segment paths are not repositories either.
	for _, repo := range out {
		require.Len(t, strings.Split(repo, "/"), 2, "unexpected repo %q", repo)
	}

	// The result is de-duplicated and keeps the curation order of the star
	// list: the entries are not sorted alphabetically, because the order
	// the user arranged them in is the only curation signal a star list
	// carries.
	require.Len(t, out, len(uniqueStrings(out)))
	for i := 1; i < len(out); i++ {
		require.NotEqual(t, out[i-1], out[i], "the result contains a duplicate")
	}
	require.Equal(t, []string{
		"davidcollom/awesomegen",
		"cli/cli",
		"golang/go",
		"BurntSushi/ripgrep",
	}, out)
}

// uniqueStrings returns the distinct values of a slice, preserving order.
func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func TestListReposPagination(t *testing.T) {
	var pages int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch page {
		case "1":
			_, _ = w.Write([]byte(`<a href="/owner/one">one</a><a rel="next" href="?page=2">Next</a>`))
		case "2":
			_, _ = w.Write([]byte(`<a href="/owner/two">two</a>`))
		default:
			_, _ = w.Write([]byte(`<html></html>`))
		}
	}))
	defer srv.Close()

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL

	out, err := sc.ListRepos(context.Background(), "owner", "list")
	require.NoError(t, err)
	require.Equal(t, []string{"owner/one", "owner/two"}, out)
	require.Equal(t, 2, pages)
}

func TestListReposRejectsInvalidInput(t *testing.T) {
	sc, _ := newServer(t, "../../testdata/stars_list_page.html")

	// A percent sign used to reach http.NewRequestWithContext as an invalid
	// URL escape, which was discarded and then dereferenced.
	for _, tc := range []struct{ user, slug string }{
		{"bad%slug", "tools"},
		{"alice\nbob", "tools"},
		{"alice", "bad%slug"},
		{"alice", "tools?x=1&y=2"},
		{"../../evil", "tools"},
		{"", "tools"},
		{"alice", ""},
	} {
		_, err := sc.ListRepos(context.Background(), tc.user, tc.slug)
		require.Error(t, err, "expected an error for user=%q slug=%q", tc.user, tc.slug)
	}
}

func TestListReposNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL

	out, err := sc.ListRepos(context.Background(), "owner", "missing")
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestListReposServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL

	_, err := sc.ListRepos(context.Background(), "owner", "list")
	require.Error(t, err)
}

func TestListReposRejectsOversizedPage(t *testing.T) {
	// A body larger than the limit must be reported as an error rather
	// than parsed as a truncated - and therefore silently incomplete - page.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>"))
		chunk := make([]byte, 64*1024)
		for i := range chunk {
			chunk[i] = 'a'
		}
		for written := 0; written < scrape.MaxPageBytes+(1<<20); written += len(chunk) {
			_, _ = w.Write(chunk)
		}
	}))
	defer srv.Close()

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL

	_, err := sc.ListRepos(context.Background(), "owner", "list")
	require.Error(t, err)
	require.Contains(t, err.Error(), "larger than")
}

// TestListReposKeepsRepositoriesWhoseOwnerIsAPageName makes sure a
// repository is not discarded because its owner has the same name as one of
// the pages of GitHub.
//
// The exclusions for the navigation links are only applied to the fallback
// scan. The rows of the list are the entries of the list, so a repository
// that is listed there is a repository.
func TestListReposKeepsRepositoriesWhoseOwnerIsAPageName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(
			`<div class="Box-row"><a href="/education/autograding-command-grader">grader</a></div>` +
				`<div class="Box-row"><a href="/features/awesome-actions">actions</a></div>`,
		))
	}))
	defer srv.Close()

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL

	out, err := sc.ListRepos(context.Background(), "owner", "list")
	require.NoError(t, err)
	require.Equal(t, []string{
		"education/autograding-command-grader",
		"features/awesome-actions",
	}, out)
}

// TestListReposFallbackScanRejectsNavigationLinks makes sure the fallback
// anchor scan - the one that is used when the markup of the rows changes -
// still rejects the links to the pages of GitHub.
func TestListReposFallbackScanRejectsNavigationLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// There is no row in this page, so the scraper falls back to the
		// general anchor scan.
		_, _ = w.Write([]byte(
			`<a href="/settings/profile">Settings</a>` +
				`<a href="/collections/some-collection">Collections</a>` +
				`<a href="/owner/repo">Repo</a>`,
		))
	}))
	defer srv.Close()

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL

	out, err := sc.ListRepos(context.Background(), "owner", "list")
	require.NoError(t, err)
	require.Equal(t, []string{"owner/repo"}, out)
}
