package render_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"

	"github.com/davidcollom/awesomegen/internal/config"
	"github.com/davidcollom/awesomegen/internal/github"
	"github.com/davidcollom/awesomegen/internal/render"
)

type fakeGH struct{ repos map[string]github.RepoMeta }

func (f fakeGH) GetRepo(_ context.Context, name string) (github.RepoMeta, error) {
	return f.repos[name], nil
}

func TestEnrichAndFilter(t *testing.T) {
	clk := clockwork.NewFakeClockAt(time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	gh := fakeGH{repos: map[string]github.RepoMeta{
		"a/b": {FullName: "a/b", Stars: 100, PushedAt: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)},
		"x/y": {FullName: "x/y", Stars: 10, PushedAt: time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC)},
	}}
	l := config.List{Title: "T", MinStars: 50, StaleMonths: ptr(24),
		Categories: []*config.Category{{Name: "Repositories", Items: []*config.Item{
			{Type: config.ItemGitHub, Repo: "a/b"},
			{Type: config.ItemGitHub, Repo: "x/y"},
		}}}}
	enr := render.Enricher{GH: gh, Now: clk}

	out, err := enr.EnrichAndFilter(context.Background(), l)
	require.NoError(t, err)
	require.Len(t, out.Categories[0].Items, 1)
	require.Equal(t, "a/b", out.Categories[0].Items[0].GHMeta.FullName)
}

// TestEnrichAndFilterStaleMonthsAreMonths makes sure the staleness window is
// measured in months and not in years: time.AddDate takes the years, the
// months and the days, in that order.
func TestEnrichAndFilterStaleMonthsAreMonths(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	clk := clockwork.NewFakeClockAt(now)
	gh := fakeGH{repos: map[string]github.RepoMeta{
		// Pushed two years and one month ago: stale for 24 months.
		"old/repo": {FullName: "old/repo", Stars: 100, PushedAt: now.AddDate(-2, -1, 0)},
		// Pushed one year and eleven months ago: still fresh for 24 months.
		"new/repo": {FullName: "new/repo", Stars: 100, PushedAt: now.AddDate(-1, -11, 0)},
	}}
	l := config.List{Title: "T", StaleMonths: ptr(24),
		Categories: []*config.Category{{Name: "Repositories", Items: []*config.Item{
			{Type: config.ItemGitHub, Repo: "old/repo"},
			{Type: config.ItemGitHub, Repo: "new/repo"},
		}}}}
	enr := render.Enricher{GH: gh, Now: clk}

	out, err := enr.EnrichAndFilter(context.Background(), l)
	require.NoError(t, err)
	require.Len(t, out.Categories[0].Items, 1)
	require.Equal(t, "new/repo", out.Categories[0].Items[0].GHMeta.FullName)
}

// cancelingGH fails every lookup, like a client whose context was canceled.
type cancelingGH struct{}

func (cancelingGH) GetRepo(ctx context.Context, _ string) (github.RepoMeta, error) {
	return github.RepoMeta{}, ctx.Err()
}

// TestEnrichAndFilterTreatsCancellationAsFatal makes sure a canceled run is
// reported as an error and not as a set of individual lookup failures: a
// partial list must never be written as if it were complete.
func TestEnrichAndFilterTreatsCancellationAsFatal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	l := config.List{Title: "T",
		Categories: []*config.Category{{Name: "Repositories", Items: []*config.Item{
			{Type: config.ItemGitHub, Repo: "a/b"},
		}}}}
	enr := render.Enricher{GH: cancelingGH{}, Now: clockwork.NewRealClock()}

	_, err := enr.EnrichAndFilter(ctx, l)
	require.ErrorIs(t, err, context.Canceled)
	var lookupErr *render.LookupError
	require.False(t, errors.As(err, &lookupErr), "a canceled run must not be reported as a lookup failure")
}

// ptr returns a pointer to a value, for the optional settings that are only
// defaulted when they were left unset.
func ptr[T any](v T) *T { return &v }

// TestEnrichAndFilterDoesNotMutateTheInput makes sure the caller's
// configuration is left alone: the enrichment writes the repository metadata
// into the items, and a second run over the same configuration would
// otherwise see the metadata of the first run.
func TestEnrichAndFilterDoesNotMutateTheInput(t *testing.T) {
	clk := clockwork.NewFakeClockAt(time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	gh := fakeGH{repos: map[string]github.RepoMeta{
		"a/b": {FullName: "a/b", URL: "https://github.com/a/b", Stars: 100, PushedAt: clk.Now().AddDate(0, -1, 0)},
	}}

	l := config.List{Title: "T",
		Categories: []*config.Category{{Name: "Repositories", Items: []*config.Item{
			{Type: config.ItemGitHub, Repo: "a/b"},
		}}}}
	enr := render.Enricher{GH: gh, Now: clk}

	out, err := enr.EnrichAndFilter(context.Background(), l)
	require.NoError(t, err)

	require.Len(t, out.Categories, 1)
	require.Len(t, out.Categories[0].Items, 1)
	require.NotNil(t, out.Categories[0].Items[0].GHMeta, "the enriched copy carries the metadata")
	require.Equal(t, "a/b", out.Categories[0].Items[0].GHMeta.FullName)

	require.Nil(t, l.Categories[0].Items[0].GHMeta, "the input must not be modified")
}

// TestEnrichAndFilterStaleMonthsZeroNeverStale makes sure an explicit
// stale_months of 0 disables the staleness check instead of being replaced
// by the default.
func TestEnrichAndFilterStaleMonthsZeroNeverStale(t *testing.T) {
	now := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)
	clk := clockwork.NewFakeClockAt(now)
	gh := fakeGH{repos: map[string]github.RepoMeta{
		"a/b": {FullName: "a/b", URL: "https://github.com/a/b", Stars: 100, PushedAt: now.AddDate(-20, 0, 0)},
	}}

	l := config.List{Title: "T", StaleMonths: ptr(0),
		Categories: []*config.Category{{Name: "Repositories", Items: []*config.Item{
			{Type: config.ItemGitHub, Repo: "a/b"},
		}}}}
	enr := render.Enricher{GH: gh, Now: clk}

	out, err := enr.EnrichAndFilter(context.Background(), l)
	require.NoError(t, err)
	require.Len(t, out.Categories[0].Items, 1,
		"stale_months: 0 disables the staleness check")
}

// TestLookupErrorAllFailed makes sure a list whose repositories all failed
// to load is told apart from a list that only holds links.
//
// The first one must not be rendered: the repositories are missing, and
// writing what is left would empty a curated README. The second one is a
// list of links, and it is rendered as it is.
func TestLookupErrorAllFailed(t *testing.T) {
	tests := []struct {
		name string
		err  *render.LookupError
		want bool
	}{
		{
			name: "every lookup failed",
			err:  &render.LookupError{Failed: 3, Total: 3},
			want: true,
		},
		{
			name: "some lookups failed",
			err:  &render.LookupError{Failed: 1, Total: 3},
			want: false,
		},
		{
			name: "the list holds links only",
			err:  &render.LookupError{Failed: 0, Total: 0},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.err.AllFailed())
		})
	}
}

// countingGH counts the lookups it is asked for.
type countingGH struct {
	repos map[string]github.RepoMeta
	calls map[string]int
}

func (c *countingGH) GetRepo(_ context.Context, name string) (github.RepoMeta, error) {
	if c.calls == nil {
		c.calls = map[string]int{}
	}
	c.calls[name]++
	return c.repos[name], nil
}

// TestEnrichAndFilterLoadsEveryRepositoryOnce makes sure that a repository
// that is curated in two categories is loaded once.
//
// The rate limit of the API is small, and a repository that is looked up
// twice is a lookup that can fail where the first one succeeded.
func TestEnrichAndFilterLoadsEveryRepositoryOnce(t *testing.T) {
	clk := clockwork.NewFakeClockAt(time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	gh := &countingGH{repos: map[string]github.RepoMeta{
		"a/b": {FullName: "a/b", Stars: 100, PushedAt: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)},
	}}
	l := config.List{Title: "T", MinStars: 50, StaleMonths: ptr(24),
		Categories: []*config.Category{
			{Name: "First", Items: []*config.Item{
				{Type: config.ItemGitHub, Repo: "a/b"},
			}},
			{Name: "Second", Items: []*config.Item{
				{Type: config.ItemGitHub, Repo: "a/b"},
			}},
		}}
	enr := render.Enricher{GH: gh, Now: clk}

	out, err := enr.EnrichAndFilter(context.Background(), l)
	require.NoError(t, err)
	require.Len(t, out.Categories, 2)
	require.Equal(t, 1, gh.calls["a/b"], "the repository is loaded once")
}
