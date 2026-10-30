package scrape_test

import (
	"context"
	"fmt"
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
	// The auxiliary links of an entry - the topics, the stargazers and the
	// forks - sit next to the name of the repository and must not be
	// mistaken for one.
	require.NotContains(t, out, "topics/awesome-list")
	require.NotContains(t, out, "topics/go")
	require.NotContains(t, out, "topics/compiler")
	require.NotContains(t, out, "topics/education")
	require.NotContains(t, out, "davidcollom/awesomegen/stargazers")
	require.NotContains(t, out, "davidcollom/awesomegen/forks")
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
		"education/autograding-command-grader",
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

// TestListReposNotFound makes sure that a star list that does not exist is
// reported as an error.
//
// A missing list and an empty list are not the same thing: a missing list
// would empty a curated README without a word, because nothing would fail.
func TestListReposNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL

	out, err := sc.ListRepos(context.Background(), "owner", "missing")
	require.Error(t, err)
	require.Empty(t, out)
}

// TestListReposEndsOnMissingPage makes sure that a page that is not there
// ends the list instead of failing the run.
//
// Only the first page proves that the list exists at all. A later page that
// is gone simply ends the list.
func TestListReposEndsOnMissingPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch page {
		case "1":
			_, _ = w.Write([]byte(`<a href="/owner/one">one</a><a rel="next" href="?page=2">Next</a>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL

	out, err := sc.ListRepos(context.Background(), "owner", "list")
	require.NoError(t, err)
	require.Equal(t, []string{"owner/one"}, out)
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
// scan. The entries of the list are the repositories of the list, and the
// names collide with real repository owners - "education" and "features",
// for example - so a repository must never be discarded because of its
// owner.
func TestListReposKeepsRepositoriesWhoseOwnerIsAPageName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(
			`<div id="user-list-repositories">` +
				`<div><h2 class="h3"><a href="/education/autograding-command-grader">` +
				`<span class="text-normal">education / </span>autograding-command-grader</a></h2></div>` +
				`<div><h2 class="h3"><a href="/features/awesome-actions">` +
				`<span class="text-normal">features / </span>awesome-actions</a></h2></div>` +
				`</div>`,
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

// TestListReposIgnoresAuxiliaryLinksOfAnEntry makes sure the links that sit
// next to the name of a repository - the topics, the stargazers and the
// forks - are not mistaken for repositories.
//
// They are two or three segment paths, so the ones that are two segments
// long would be accepted by the general scan. The entries of the list are
// selected specifically, so they are never collected.
func TestListReposIgnoresAuxiliaryLinksOfAnEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(
			`<div id="user-list-repositories">` +
				`<div>` +
				`<h2 class="h3"><a href="/owner/repo"><span class="text-normal">owner / </span>repo</a></h2>` +
				`<a class="topic-tag" href="/topics/go">go</a>` +
				`<a class="Link--muted" href="/owner/repo/stargazers">42</a>` +
				`<a class="Link--muted" href="/owner/repo/forks">3</a>` +
				`</div>` +
				`</div>`,
		))
	}))
	defer srv.Close()

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL

	out, err := sc.ListRepos(context.Background(), "owner", "list")
	require.NoError(t, err)
	require.Equal(t, []string{"owner/repo"}, out)
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

// TestListReposStopsAtThePageCap makes sure that the scraper stops at the
// page cap and does not ask for more pages than it is allowed to.
//
// The cap is the last resort against a list that never ends, and it has to
// be the number of pages it claims to be.
func TestListReposStopsAtThePageCap(t *testing.T) {
	var pages int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<a href="/owner/repo%d">repo</a><a rel="next" href="?page=2">Next</a>`, pages)
	}))
	defer srv.Close()

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL

	out, err := sc.ListRepos(context.Background(), "owner", "list")
	require.NoError(t, err)
	require.Len(t, out, scrape.MaxPages)
	require.Equal(t, scrape.MaxPages, pages, "the scraper asks for at most MaxPages pages")
}

// TestListReposIgnoresPathsThatAreNotRepositories makes sure that the
// fallback scan only collects paths that look like a repository.
//
// The fallback is used when GitHub changes the markup of the list, and then
// every link of the page is a candidate.
func TestListReposIgnoresPathsThatAreNotRepositories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<a href="/owner/one">one</a>` +
			`<a href="/not a repo/two">two</a>` +
			`<a href="/owner/three/extra">three</a>` +
			`<a href="/..">up</a>`))
	}))
	defer srv.Close()

	sc := scrape.NewScraper(stdr.New(nil))
	sc.BaseURL = srv.URL

	out, err := sc.ListRepos(context.Background(), "owner", "list")
	require.NoError(t, err)
	require.Equal(t, []string{"owner/one"}, out)
}
