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
	for page := 1; page < MaxPages; page++ { // reasonable safety cap
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
			break
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

		// The entries of the list are rendered as rows, so the rows are
		// the repositories. Only the fallback scan has to tell the
		// navigation links apart from the repositories.
		countBefore := len(out)
		hrefs, fromRows := repoHrefs(doc)
		for _, href := range hrefs {
			repo, ok := parseRepoHref(href, fromRows)
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

		if added == 0 || !hasNext(doc) {
			return out, nil
		}
	}

	// The page cap was reached while the list still had more pages.
	s.log.Info("page cap reached, the list may be truncated", "max_pages", MaxPages-1)
	// The order of the repositories is the curation order of the star list
	// and is deliberately not sorted.
	return out, nil
}

// repoHrefs returns the hrefs of the anchors that may point at a
// repository, and whether they were taken from the rows of the list.
//
// GitHub renders the entries of a star list as rows, so the row selectors
// are preferred. The general anchor scan is kept as a fallback so a change
// to the markup does not silently produce an empty list.
func repoHrefs(doc *goquery.Document) ([]string, bool) {
	sel := doc.Find(`div.Box-row a[href^="/"], article.Box-row a[href^="/"]`)
	fromRows := sel.Length() > 0
	if !fromRows {
		sel = doc.Find(`a[href^="/"]`)
	}
	var hrefs []string
	sel.Each(func(_ int, a *goquery.Selection) {
		if href, ok := a.Attr("href"); ok {
			hrefs = append(hrefs, href)
		}
	})
	return hrefs, fromRows
}

// parseRepoHref returns the owner/repo of a link that points at a
// repository. Links with a query string or a fragment address a GitHub
// page rather than a repository.
//
// The navigation exclusions are only applied to the fallback scan. The rows
// of the list are the entries of the list, so a repository whose owner
// happens to have the same name as one of the pages of GitHub - "features"
// or "education", for example - is a repository and has to be kept.
func parseRepoHref(href string, fromRows bool) (string, bool) {
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
	if !fromRows && slices.Contains(excludedLinks, strings.ToLower(parts[0])) {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}

// hasNext reports whether the page advertises a further page.
//
// The pagination control is matched precisely. The previous implementation
// matched any anchor whose text contained "next", which was true for the
// site banner on every page and therefore never stopped the loop.
func hasNext(doc *goquery.Document) bool {
	if doc.Find(`a[rel="next"]`).Length() > 0 {
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
