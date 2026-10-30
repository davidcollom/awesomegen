package scrape

import (
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

var excludedLinks = []string{
	"topics",
	"marketplace",
	"orgs",
	"contact",
	"solutions",
	"sponsors",
	"resources",
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
		doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, MaxPageBytes))
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}

		// conservative anchor scan: find /owner/repo links
		countBefore := len(out)
		doc.Find(`a[href^="/"]`).Each(func(_ int, a *goquery.Selection) {
			href, _ := a.Attr("href")
			href = strings.Trim(href, "/")
			parts := strings.Split(href, "/")
			// If we have exactly 2 `/`'s and contains a path/part from `excludedLinks`
			if len(parts) == 2 && !slices.Contains(excludedLinks, parts[0]) {
				slug := parts[0] + "/" + parts[1]
				if _, ok := seen[slug]; !ok {
					seen[slug] = struct{}{}
					out = append(out, slug)
				}
			}
		})
		added := len(out) - countBefore
		s.log.Info("scraped page", "url", u, "repos_found", added)

		if added == 0 || !hasNext(doc) {
			break
		}
	}

	slices.Sort(out)
	return out, nil
}

func hasNext(doc *goquery.Document) bool {
	next := false
	doc.Find("a").Each(func(_ int, a *goquery.Selection) {
		if strings.Contains(strings.ToLower(a.Text()), "next") {
			next = true
		}
	})
	return next
}
