package scrape

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-logr/logr"
)

type Scraper interface {
	ListRepos(ctx context.Context, user, slug string) ([]string, error)
}

type HTMLScraper struct {
	http *http.Client
	log  logr.Logger
}

func NewScraper(log logr.Logger) *HTMLScraper {
	return &HTMLScraper{
		http: &http.Client{Timeout: 15 * time.Second},
		log:  log,
	}
}

func (s *HTMLScraper) ListRepos(ctx context.Context, user, slug string) ([]string, error) {
	var out []string
	seen := map[string]struct{}{}
	for page := 1; page < 50; page++ { // reasonable safety cap
		u := fmt.Sprintf("https://github.com/stars/%s/lists/%s?page=%d", user, slug, page)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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
		doc, err := goquery.NewDocumentFromReader(resp.Body)
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
			if len(parts) == 2 && parts[0] != "topics" && parts[0] != "marketplace" && parts[0] != "orgs" {
				slug := parts[0] + "/" + parts[1]
				if _, ok := seen[slug]; !ok {
					seen[slug] = struct{}{}
					out = append(out, slug)
				}
			}
		})
		added := len(out) - countBefore
		s.log.V(1).Info("scraped page", "url", u, "repos_found", added)

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
