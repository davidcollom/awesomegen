package scrape

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

func TestRetryBackoff(t *testing.T) {
	client := newHTTPClient(logr.Discard())
	require.Equal(t, 10, client.RetryMax)
	require.Equal(t, 15*time.Second, client.HTTPClient.Timeout)
	for i, want := range []time.Duration{2, 4, 8, 16, 32, 64, 120, 120, 120, 120} {
		require.Equal(t, want*time.Second, client.Backoff(client.RetryWaitMin, client.RetryWaitMax, i, nil))
	}
	response := &http.Response{StatusCode: 503, Header: http.Header{"Retry-After": {"180"}}}
	require.Equal(t, 3*time.Minute, client.Backoff(client.RetryWaitMin, client.RetryWaitMax, 0, response))
	require.Equal(t, 10*time.Minute, NewScraper(logr.Discard()).http.Timeout)
}

func TestListReposRetriesTransientFailures(t *testing.T) {
	for _, status := range []int{429, 500, 502, 503, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			counts := map[string]int{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := r.URL.Query().Get("page")
				counts[page]++
				if counts[page] < 3 {
					w.WriteHeader(status)
					return
				}
				if page == "1" {
					fmt.Fprint(w, `<a href="/owner/one">one</a><a rel="next" href="?page=2">Next</a>`)
					return
				}
				fmt.Fprint(w, `<a href="/owner/two">two</a>`)
			}))
			defer srv.Close()
			client := newHTTPClient(logr.Discard())
			client.Backoff = func(time.Duration, time.Duration, int, *http.Response) time.Duration { return 0 }
			sc := NewScraper(logr.Discard())
			sc.http = client.StandardClient()
			sc.BaseURL = srv.URL
			repos, err := sc.ListRepos(context.Background(), "owner", "list")
			require.NoError(t, err)
			require.Equal(t, []string{"owner/one", "owner/two"}, repos)
			require.Equal(t, map[string]int{"1": 3, "2": 3}, counts)
		})
	}
}

func TestListReposRetryExhaustionAndPermanentErrors(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 501, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(status) }))
			defer srv.Close()
			client := newHTTPClient(logr.Discard())
			client.Backoff = func(time.Duration, time.Duration, int, *http.Response) time.Duration { return 0 }
			sc := NewScraper(logr.Discard())
			sc.http = client.StandardClient()
			sc.BaseURL = srv.URL
			repos, err := sc.ListRepos(context.Background(), "owner", "list")
			require.Error(t, err)
			require.Nil(t, repos)
			want := 1
			if status == 504 {
				want = 11
			}
			require.Equal(t, want, calls)
		})
	}
}

func TestListReposCancelsBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Header().Set("Retry-After", "180"); w.WriteHeader(503) }))
	defer srv.Close()
	client := newHTTPClient(logr.Discard())
	client.Backoff = func(time.Duration, time.Duration, int, *http.Response) time.Duration {
		cancel()
		return 3 * time.Minute
	}
	sc := NewScraper(logr.Discard())
	sc.http = client.StandardClient()
	sc.BaseURL = srv.URL
	_, err := sc.ListRepos(ctx, "owner", "list")
	require.True(t, errors.Is(err, context.Canceled), "got %v", err)
}

func TestListReposRetriesConnectionFailure(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
		fmt.Fprint(w, `<a href="/owner/one">one</a>`)
	}))
	defer srv.Close()
	client := newHTTPClient(logr.Discard())
	client.Backoff = func(time.Duration, time.Duration, int, *http.Response) time.Duration { return 0 }
	sc := NewScraper(logr.Discard())
	sc.http = client.StandardClient()
	sc.BaseURL = srv.URL
	repos, err := sc.ListRepos(context.Background(), "owner", "list")
	require.NoError(t, err)
	require.Equal(t, []string{"owner/one"}, repos)
	require.Equal(t, 2, calls)
}

func TestListReposBoundsRetryAfterWait(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Retry-After", "180")
		w.WriteHeader(503)
	}))
	defer srv.Close()
	sc := NewScraper(logr.Discard())
	sc.http.Timeout = 50 * time.Millisecond
	sc.BaseURL = srv.URL
	_, err := sc.ListRepos(context.Background(), "owner", "list")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, calls)
}
