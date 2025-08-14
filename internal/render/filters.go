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
		var items []config.Item
		for _, it := range out.Categories[ci].Items {
			if it.Type == config.ItemGitHub {
				meta, err := e.GH.GetRepo(ctx, it.Repo)
				if err != nil {
					e.Log.V(1).Info("github lookup failed", "repo", it.Repo, "err", err)
					continue
				}
				if meta.Archived {
					continue
				}
				if meta.Stars < out.MinStars {
					continue
				}
				if stale(meta.PushedAt, e.Now.Now(), out.StaleMonths) {
					continue
				}
				it.GHMeta = &meta
			}
			items = append(items, it)
		}
		sortItems(items)
		out.Categories[ci].Items = items
	}
	return out, nil
}

func stale(pushed, now time.Time, months int) bool {
	dm := (now.Year()-pushed.Year())*12 + int(now.Month()-pushed.Month())
	return dm > months
}
