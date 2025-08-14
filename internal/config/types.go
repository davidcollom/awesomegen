package config

import "github.com/davidcollom/awesomegen/internal/github"

type Config struct {
	Version int    `yaml:"version"`
	User    string `yaml:"user"`
	Lists   []List `yaml:"lists"`
}

type List struct {
	Slug        string     `yaml:"slug"`
	Title       string     `yaml:"title"`
	Tagline     string     `yaml:"tagline"`
	Output      string     `yaml:"output"`
	MinStars    int        `yaml:"min_stars"`
	StaleMonths int        `yaml:"stale_months"`
	Badges      []string   `yaml:"badges"`
	Categories  []Category `yaml:"categories"` // usually one auto category "Repositories"
}

type Category struct {
	Name  string `yaml:"name"`
	Items []Item `yaml:"items"`
}

type ItemType string

const (
	ItemGitHub ItemType = "github"
	ItemLink   ItemType = "link"
)

type Item struct {
	Type   ItemType         `yaml:"type"`
	Repo   string           `yaml:"repo,omitempty"` // owner/repo for github items
	URL    string           `yaml:"url,omitempty"`  // for link items
	Title  string           `yaml:"title,omitempty"`
	Notes  string           `yaml:"notes,omitempty"`
	GHMeta *github.RepoMeta `yaml:"-"`
}

// SeedRepos converts a flat list of owner/repo strings into one Category named "Repositories".
func (l *List) SeedRepos(slugs []string) {
	items := make([]Item, 0, len(slugs))
	for _, r := range slugs {
		items = append(items, Item{Type: ItemGitHub, Repo: r})
	}
	if len(l.Categories) == 0 {
		l.Categories = []Category{{Name: "Repositories", Items: items}}
		return
	}
	// replace first category if exists
	l.Categories[0].Name = "Repositories"
	l.Categories[0].Items = items
}
