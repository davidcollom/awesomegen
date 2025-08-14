package render_test

import (
	"context"
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
		Categories: []config.Category{{Name: "Repositories", Items: []config.Item{
			{Type: config.ItemGitHub, Repo: "a/b"},
			{Type: config.ItemGitHub, Repo: "x/y"},
		}}}}
	enr := render.Enricher{GH: gh, Now: clk}

	out, err := enr.EnrichAndFilter(context.Background(), l)
	require.NoError(t, err)
	require.Len(t, out.Categories[0].Items, 1)
	require.Equal(t, "a/b", out.Categories[0].Items[0].GHMeta.FullName)
}
