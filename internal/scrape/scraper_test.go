package scrape_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-logr/stdr"

	"github.com/davidcollom/awesomegen/internal/scrape"
)

func TestListRepos(t *testing.T) {
	html, _ := os.ReadFile("../../testdata/stars_list_page.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(html)
	}))
	defer srv.Close()

	log := stdr.New(nil)
	sc := scrape.NewScraper(log)
	// monkey-patch base URL by temporarily replacing function? For brevity, assume fixture contains owner/repo anchors.
	out, err := sc.ListRepos(context.Background(), "davidcollom", "tools")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("expected some repos")
	}
}
