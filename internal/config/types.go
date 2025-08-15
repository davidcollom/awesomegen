package config

import "github.com/davidcollom/awesomegen/internal/github"

type Config struct {
	Version int    `yaml:"version"` // Configuration file version
	User    string `yaml:"user"`    // GitHub username or owner
	Lists   []List `yaml:"lists"`   // List of repository lists to generate
}

type List struct {
	Slug        string     `yaml:"slug"`         // Unique identifier for the list
	Sources     []Source   `yaml:"sources"`      // List of sources (users/repos) to include
	Title       string     `yaml:"title"`        // Title of the list
	Tagline     string     `yaml:"tagline"`      // Short description or tagline
	Output      string     `yaml:"output"`       // Output file path or name
	MinStars    int        `yaml:"min_stars"`    // Minimum GitHub stars required for inclusion
	StaleMonths int        `yaml:"stale_months"` // Months since last update to consider stale
	Badges      []string   `yaml:"badges"`       // List of badge names to display
	Categories  []Category `yaml:"categories"`   // List categories; usually one auto category "Repositories"

	GroupByTopic      bool   `yaml:"group_by_topic"`      // Group repositories by GitHub topic (default: false)
	TopicFallback     string `yaml:"topic_fallback"`      // Fallback topic name if none found (default: "misc")
	TopicGroupingMode string `yaml:"topic_grouping_mode"` // Topic grouping style: "flat" or "nested" (default: "flat")
	StarsFormat       string `yaml:"stars_format"`        // Format for displaying stars: "locale", "compact", "none" (default: "locale")
	Locale            string `yaml:"locale"`              // Locale for formatting numbers, BCP-47 (default: "en-GB")

	GroupByTopTags bool              `yaml:"group_by_top_tags"` // Group repositories by top tags
	TopTagsLimit   int               `yaml:"top_tags_limit"`    // Maximum number of top tags to group by
	MinTagCount    int               `yaml:"min_tag_count"`     // Minimum count for a tag to be considered a top tag
	SingleHome     bool              `yaml:"single_home"`       // If true, generate a single home page
	TagAliases     map[string]string `yaml:"tag_aliases"`       // Map of tag aliases for normalization
}

type Source struct {
	User string `yaml:"user"` // GitHub username or organization
	Slug string `yaml:"slug"` // Repository name or slug
}

type Category struct {
	Name  string `yaml:"name"`  // Category name (e.g., "Repositories")
	Items []Item `yaml:"items"` // Items belonging to this category
}

type ItemType string

// ItemType represents the type of item (GitHub repo or external link)
const (
	ItemGitHub ItemType = "github" // GitHub repository item
	ItemLink   ItemType = "link"   // External link item
)

type Item struct {
	Type   ItemType         `yaml:"type"`            // Type of item: "github" or "link"
	Repo   string           `yaml:"repo,omitempty"`  // GitHub owner/repo (for GitHub items)
	URL    string           `yaml:"url,omitempty"`   // External URL (for link items)
	Title  string           `yaml:"title,omitempty"` // Display title for the item
	Notes  string           `yaml:"notes,omitempty"` // Additional notes or description
	GHMeta *github.RepoMeta `yaml:"-"`               // GitHub metadata (populated at runtime)
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
