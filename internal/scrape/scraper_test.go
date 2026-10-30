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

	// The result is sorted and de-duplicated.
	require.IsIncreasing(t, out)
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
