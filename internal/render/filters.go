package render

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/jonboulle/clockwork"

	"github.com/davidcollom/awesomegen/internal/config"
	"github.com/davidcollom/awesomegen/internal/github"
)

type Enricher struct {
	GH  github.Client
	Now clockwork.Clock
	Log logr.Logger
}

func (e Enricher) EnrichAndFilter(ctx context.Context, list config.List) (config.List, error) {
	out := list
	for ci := range out.Categories {
		e.Log.Info("enriching category", "name", out.Categories[ci].Name, "items", len(out.Categories[ci].Items))
		var items []*config.Item
		for _, it := range out.Categories[ci].Items {
			if it.Type == config.ItemGitHub {
				meta, err := e.GH.GetRepo(ctx, it.Repo)
				if err != nil {
					e.Log.Error(err, "github lookup failed", "repo", it.Repo)
					continue
				}
				if meta.Archived {
					e.Log.Info("skipping archived repo", "repo", it.Repo)
					continue
				}
				if meta.Stars < out.MinStars {
					e.Log.Info("skipping low-star repo", "repo", it.Repo, "stars", meta.Stars, "minStars", out.MinStars)
					continue
				}
				if stale(meta.PushedAt, e.Now.Now(), out.StaleMonths) {
					e.Log.Info("skipping stale repo", "repo", it.Repo, "pushedAt", meta.PushedAt, "staleMonths", out.StaleMonths)
					continue
				}
				it.GHMeta = &meta
			}
			items = append(items, it)
		}
		sortItems(items)
		out.Categories[ci].Items = items
		e.Log.Info("enriched category", "name", out.Categories[ci].Name, "items", len(out.Categories[ci].Items))
	}
	return out, nil
}

func stale(pushed, now time.Time, months int) bool {
	dm := (now.Year()-pushed.Year())*12 + int(now.Month()-pushed.Month())
	return dm > months
}
