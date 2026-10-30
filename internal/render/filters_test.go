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
	l := config.List{Title: "T", MinStars: 50, StaleMonths: 24,
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
	l := config.List{Title: "T", StaleMonths: 24,
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
