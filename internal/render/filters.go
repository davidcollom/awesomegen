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

// LookupError reports that one or more repository lookups failed because the
// repository does not exist. Callers can use errors.As to detect it and
// decide whether to continue.
//
// The repositories that are reported here were deleted, renamed or made
// private. The curation of the list is the source of truth, so dropping them
// keeps the list intact. Every other failure is reported as an
// *UnavailableError instead, because it leaves the list unknown.
type LookupError struct {
	Failed int
	Total  int
	Err    error
}

func (e *LookupError) Error() string {
	return fmt.Sprintf("%d of %d repositories could not be loaded (last error: %v)", e.Failed, e.Total, e.Err)
}

func (e *LookupError) Unwrap() error { return e.Err }

// UnavailableError reports a repository lookup that failed for a reason which
// leaves the list unknown rather than curated.
//
// A repository that does not exist any more is dropped from the list and
// reported as a *LookupError: the curation is the source of truth, and it
// does not name the repository any more. Every other failure - an expired
// token, a rate limit, a server error, a network problem - says nothing
// about the repositories of the list. The list is then unknown, and it must
// not be rendered: writing it would replace a curated README with a list
// that is missing entries without a word.
//
// The error is deliberately not a *LookupError, so that the callers stop the
// run instead of continuing with what is left.
type UnavailableError struct {
	Repo string
	Err  error
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("the repository %s could not be loaded (%v); the list is unknown rather than curated and is not rendered",
		e.Repo, e.Err)
}

func (e *UnavailableError) Unwrap() error { return e.Err }

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
// A repository that does not exist any more is skipped, and the number of
// such repositories is reported as a *LookupError, so that the callers can
// decide whether a partially enriched list is acceptable.
//
// A lookup that fails for any other reason stops the enrichment with an
// *UnavailableError: the list is then unknown rather than curated, and it
// must not be rendered.
//
// The supplied list is not modified: the enriched copy is returned.
func (e Enricher) EnrichAndFilter(ctx context.Context, list config.List) (config.List, error) {
	if e.Now == nil {
		e.Now = clockwork.NewRealClock()
	}
	out := cloneList(list)

	// The same repository can be curated in several categories and be part
	// of a scraped star list as well. Every repository is loaded once, so
	// that a list does not spend the rate limit of the API on the same
	// repository twice.
	type result struct {
		meta github.RepoMeta
		err  error
	}
	cache := map[string]result{}

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
				res, cached := cache[it.Repo]
				if !cached {
					meta, err := e.GH.GetRepo(ctx, it.Repo)
					res = result{meta: meta, err: err}
					cache[it.Repo] = res
				}
				if res.err != nil {
					// A canceled run is not a per-repository failure: the
					// remaining lookups would fail for the same reason.
					// Report it as a fatal error so a partial list is
					// never written as if it were complete.
					if ctxErr := ctx.Err(); ctxErr != nil {
						return out, ctxErr
					}
					e.Log.Error(res.err, "github lookup failed", "repo", it.Repo)
					if !github.IsNotFound(res.err) {
						// The repository may well be there: the lookup
						// is what failed, because the token expired,
						// the API is rate limiting or down, or the
						// connection broke. Nothing says which
						// repositories of the list are missing, so
						// the list is unknown rather than curated
						// and the run stops here.
						return out, &UnavailableError{Repo: it.Repo, Err: res.err}
					}
					// The repository is gone. The entry is dropped and
					// the failure is reported, so that a deleted
					// repository does not empty a curated list
					// without a word.
					failures++
					lastErr = res.err
					continue
				}
				meta := res.meta
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
