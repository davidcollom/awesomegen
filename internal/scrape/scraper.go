package scrape

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-logr/logr"

	"github.com/davidcollom/awesomegen/internal/github"
)

// MaxPageBytes bounds the size of a single scraped page. Go's transport
// transparently decompresses responses, so without a limit a small
// compressed body can expand into an arbitrarily large document.
const MaxPageBytes = 8 << 20 // 8 MiB

// MaxPages caps the pagination loop.
const MaxPages = 50

// GitHub usernames and star-list slugs are restricted to a conservative
// charset. This keeps them from being interpreted as URL syntax when they
// are interpolated into the scrape URL.
var (
	userPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	slugPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

type Scraper interface {
	ListRepos(ctx context.Context, user, slug string) ([]string, error)
}

type HTMLScraper struct {
	http *http.Client
	log  logr.Logger
	// BaseURL is the root of the Star Lists pages. It is overridable so the
	// scraper can be tested against a local server instead of github.com.
	BaseURL string
}

func NewScraper(log logr.Logger) *HTMLScraper {
	return &HTMLScraper{
		http:    &http.Client{Timeout: 15 * time.Second},
		log:     log,
		BaseURL: "https://github.com",
	}
}

// excludedLinks lists the first path segments GitHub uses for its own
// pages. A two-segment link whose first segment is one of these is a
// navigation link, not a repository.
//
// The list is only applied to the fallback anchor scan. The rows of the
// list are the entries of the list, and the names collide with real
// repository owners - "features" and "education", for example - so a
// repository must never be discarded because of its owner.
var excludedLinks = []string{
	"about",
	"account",
	"apps",
	"blog",
	"codespaces",
	"collections",
	"contact",
	"customer-stories",
	"dashboard",
	"education",
	"enterprise",
	"events",
	"explore",
	"features",
	"gist",
	"issues",
	"join",
	"login",
	"logout",
	"marketplace",
	"new",
	"nonprofit",
	"notifications",
	"orgs",
	"organizations",
	"pricing",
	"pulls",
	"readme",
	"resources",
	"search",
	"security",
	"settings",
	"site",
	"solutions",
	"sponsors",
	"stars",
	"teams",
	"topics",
	"trends",
	"users",
	"watch",
}

func (s *HTMLScraper) ListRepos(ctx context.Context, user, slug string) ([]string, error) {
	if !userPattern.MatchString(user) {
		return nil, fmt.Errorf("scrape: invalid user %q", user)
	}
	if !slugPattern.MatchString(slug) {
		return nil, fmt.Errorf("scrape: invalid list slug %q", slug)
	}

	base := s.BaseURL
	if base == "" {
		base = "https://github.com"
	}

	var out []string
	seen := map[string]struct{}{}
	for page := 1; page <= MaxPages; page++ { // reasonable safety cap
		u := fmt.Sprintf("%s/stars/%s/lists/%s?page=%d",
			strings.TrimRight(base, "/"),
			url.PathEscape(user),
			url.PathEscape(slug),
			page,
		)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, fmt.Errorf("scrape: build request: %w", err)
		}
		req.Header.Set("User-Agent", "awesomegen (+https://github.com/davidcollom/awesomegen)")
		resp, err := s.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			// The first page of a star list that does not exist is a
			// missing list, not an empty one. The run stops with an
			// error, so that a renamed or deleted list cannot empty a
			// curated README.
			if page == 1 {
				return nil, fmt.Errorf("scrape: star list %s/%s not found", user, slug)
			}
			// A later page that is not there ends the list.
			s.log.Info("the list ended", "page", page, "repos_found", len(out))
			return out, nil
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("scrape: %s", resp.Status)
		}
		// Read one byte more than the limit so an oversized body can be
		// detected. io.LimitReader reports the limit as an ordinary EOF,
		// which the HTML parser would accept as a complete page - silently
		// dropping the repositories (and the pagination link) beyond the
		// cutoff.
		body, err := io.ReadAll(io.LimitReader(resp.Body, MaxPageBytes+1))
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("scrape: read page %d: %w", page, err)
		}
		if len(body) > MaxPageBytes {
			return nil, fmt.Errorf("scrape: page %d is larger than the %d byte limit", page, MaxPageBytes)
		}
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}

		// The entries of the list are the repositories of the list. Only
		// the fallback scan has to tell the navigation links apart from
		// the repositories.
		countBefore := len(out)
		hrefs, fromEntries := repoHrefs(doc)
		for _, href := range hrefs {
			repo, ok := parseRepoHref(href, fromEntries)
			if !ok {
				continue
			}
			if _, ok := seen[repo]; ok {
				continue
			}
			seen[repo] = struct{}{}
			out = append(out, repo)
		}
		added := len(out) - countBefore
		s.log.Info("scraped page", "url", u, "repos_found", added)

		if added == 0 {
			// A page without a new repository is the end of the list.
			// The first page cannot be empty: a star list always holds
			// at least one entry, so an empty first page means that the
			// entries of the list were not found - the layout of the
			// page changed. The run stops with an error instead of
			// writing a list that lost every repository.
			if page == 1 {
				return nil, fmt.Errorf("scrape: the first page of the star list %s/%s holds no repository; the layout of the page may have changed", user, slug)
			}
			// A page that still advertises further pages cannot be the
			// end of the list: the entries of the list are missing, and
			// the repositories that are on the pages that were not read
			// would be dropped without a word.
			if hasNext(doc) {
				return nil, fmt.Errorf("scrape: page %d of the star list %s/%s holds no repository but advertises further pages; the layout of the page may have changed", page, user, slug)
			}
			return out, nil
		}
		if !hasNext(doc) {
			return out, nil
		}
	}

	// The page cap was reached while the list still had more pages. The
	// list would be truncated, and the repositories on the pages that were
	// not read would be dropped without a word, so the run stops with an
	// error instead.
	//
	// The order of the repositories is the curation order of the star list
	// and is deliberately not sorted.
	return nil, fmt.Errorf("scrape: the star list %s/%s has more than %d pages; refusing to write a truncated list", user, slug, MaxPages)
}

// repoHrefs returns the hrefs of the links that point at the repositories of
// the list, and whether they were taken from the entries of the list.
//
// GitHub renders the entries of a star list as headings inside the
// "user-list-repositories" container. The auxiliary links of an entry - the
// stargazers, the forks and the topics - sit next to that heading, so the
// headings are selected specifically: an auxiliary link must never be
// mistaken for a repository.
//
// The general anchor scan is kept as a fallback, so that a change of the
// markup does not silently produce an empty list.
func repoHrefs(doc *goquery.Document) ([]string, bool) {
	if sel := doc.Find(`#user-list-repositories h2 a[href^="/"]`); sel.Length() > 0 {
		var hrefs []string
		sel.Each(func(_ int, a *goquery.Selection) {
			if href, ok := a.Attr("href"); ok {
				hrefs = append(hrefs, href)
			}
		})
		return hrefs, true
	}

	sel := doc.Find(`a[href^="/"]`)
	var hrefs []string
	sel.Each(func(_ int, a *goquery.Selection) {
		if href, ok := a.Attr("href"); ok {
			hrefs = append(hrefs, href)
		}
	})
	return hrefs, false
}

// parseRepoHref returns the owner/repo of a link that points at a
// repository. Links with a query string or a fragment address a GitHub
// page rather than a repository.
//
// The navigation exclusions are only applied to the fallback scan. The
// entries of the list are the repositories of the list, and the names of
// the pages of GitHub collide with real repository owners - "education" and
// "features", for example - so a repository must never be discarded because
// of its owner.
//
// The two segments have to look like an owner and a repository, so that the
// fallback scan cannot pick up a path that only happens to have two
// segments.
func parseRepoHref(href string, fromEntries bool) (string, bool) {
	if strings.ContainsAny(href, "?#") {
		return "", false
	}
	parts := strings.Split(strings.Trim(href, "/"), "/")
	if len(parts) != 2 {
		return "", false
	}
	if parts[0] == "" || parts[1] == "" {
		return "", false
	}
	if !fromEntries && slices.Contains(excludedLinks, strings.ToLower(parts[0])) {
		return "", false
	}
	if _, _, err := github.SplitOwnerRepo(parts[0] + "/" + parts[1]); err != nil {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}

// hasNext reports whether the page advertises a further page.
//
// The pagination of a star list is a container of links to the pages of the
// list, and the link to the next page carries rel="next". The previous
// implementation matched any anchor whose text contained "next", which was
// true for the site banner on every page and therefore never stopped the
// loop.
func hasNext(doc *goquery.Document) bool {
	if doc.Find(`.paginate-container a[rel="next"], .paginate-container a.next_page`).Length() > 0 {
		return true
	}
	next := false
	doc.Find("a").Each(func(_ int, a *goquery.Selection) {
		text := strings.ToLower(strings.TrimSpace(a.Text()))
		switch text {
		case "next", "more", "older", "newer":
		default:
			return
		}
		href, _ := a.Attr("href")
		if strings.Contains(href, "page=") {
			next = true
		}
	})
	return next
}
