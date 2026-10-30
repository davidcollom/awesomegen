package render

import (
	"context"
	"fmt"
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

// LookupError reports that one or more repository lookups failed. Callers can
// use errors.As to detect it and decide whether to continue.
type LookupError struct {
	Failed int
	Total  int
	Err    error
}

func (e *LookupError) Error() string {
	return fmt.Sprintf("%d of %d repository lookups failed (last error: %v)", e.Failed, e.Total, e.Err)
}

func (e *LookupError) Unwrap() error { return e.Err }

// AllFailed reports whether every repository lookup of the list failed.
//
// A list whose repositories all failed to load cannot be rendered: the
// entries are missing, and writing what is left - the curated links alone,
// if there are any - would empty a README that is still worth keeping.
//
// A list without a single repository is not a failure: it holds links only,
// and those are rendered.
func (e *LookupError) AllFailed() bool {
	return e != nil && e.Total > 0 && e.Failed == e.Total
}

// EnrichAndFilter enriches every GitHub item in the list with repository
// metadata and drops the items that fail the configured filters.
//
// Items that cannot be enriched are skipped, and the number of failures is
// reported as a *LookupError so callers can decide whether a partially
// enriched list is acceptable.
//
// The supplied list is not modified: the enriched copy is returned.
func (e Enricher) EnrichAndFilter(ctx context.Context, list config.List) (config.List, error) {
	if e.Now == nil {
		e.Now = clockwork.NewRealClock()
	}
	out := cloneList(list)

	var failures int
	var total int
	var lastErr error
	for ci := range out.Categories {
		if out.Categories[ci] == nil {
			continue
		}
		e.Log.Info("enriching category", "name", out.Categories[ci].Name, "items", len(out.Categories[ci].Items))
		var items []*config.Item
		for _, it := range out.Categories[ci].Items {
			if it == nil {
				continue
			}
			if it.IsGitHub() {
				total++
				meta, err := e.GH.GetRepo(ctx, it.Repo)
				if err != nil {
					// A canceled run is not a per-repository failure: the
					// remaining lookups would fail for the same reason.
					// Report it as a fatal error so a partial list is
					// never written as if it were complete.
					if ctxErr := ctx.Err(); ctxErr != nil {
						return out, ctxErr
					}
					e.Log.Error(err, "github lookup failed", "repo", it.Repo)
					failures++
					lastErr = err
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
				if stale(meta.PushedAt, e.Now.Now(), out.StaleMonthsValue()) {
					e.Log.Info("skipping stale repo", "repo", it.Repo, "pushedAt", meta.PushedAt, "staleMonths", out.StaleMonthsValue())
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
	if failures > 0 {
		return out, &LookupError{Failed: failures, Total: total, Err: lastErr}
	}
	return out, nil
}

// cloneList returns a deep copy of the categories and items of a list, so
// enriching it never modifies the caller's configuration.
func cloneList(list config.List) config.List {
	out := list
	if list.Categories == nil {
		return out
	}
	cats := make([]*config.Category, len(list.Categories))
	for i, c := range list.Categories {
		if c == nil {
			continue
		}
		cc := *c
		if c.Items != nil {
			items := make([]*config.Item, len(c.Items))
			for j, it := range c.Items {
				if it == nil {
					continue
				}
				cp := *it
				items[j] = &cp
			}
			cc.Items = items
		}
		cats[i] = &cc
	}
	out.Categories = cats
	return out
}

// stale reports whether a repository has not been pushed to within the given
// number of calendar months. A months value of zero or less disables the
// check.
//
// The months are added as months and not as years: AddDate takes the years,
// the months and the days, in that order.
func stale(pushed, now time.Time, months int) bool {
	if months <= 0 {
		return false
	}
	return now.After(pushed.AddDate(0, months, 0))
}
