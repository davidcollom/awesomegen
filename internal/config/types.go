//go:generate go run -tags jsonschema generate_schema.go

package config

import (
	"github.com/davidcollom/awesomegen/internal/github"
)

type Config struct {
	Version int    `yaml:"version" json:"version,omitempty"` // Configuration file version
	User    string `yaml:"user" json:"user,omitempty"`       // GitHub username or owner
	Lists   []List `yaml:"lists" json:"lists"`               // List of repository lists to generate
}

type List struct {
	// Unique identifier for the list
	Slug string `yaml:"slug" json:"slug,omitempty" jsonschema:"description=List slug identifier"`
	// List of additional sources (users/repos) to include (not in the list)
	Sources     []*Source   `yaml:"sources,omitempty" json:"sources,omitempty" jsonschema:"description=Array of additional sources to include"`
	Title       string      `yaml:"title" json:"title" jsonschema:"required,description=List title"`                                                                                    // Title of the list
	Tagline     string      `yaml:"tagline" json:"tagline,omitempty" jsonschema:"description=Short description or tagline"`                                                             // Short description or tagline
	Output      string      `yaml:"output,omitempty" json:"output,omitempty" jsonschema:"description=Output path relative to the config file"`                                          // Output file path or name
	MinStars    int         `yaml:"min_stars,omitempty" json:"min_stars,omitempty" jsonschema:"description=Minimum stars required to be included"`                                      // Minimum GitHub stars required for inclusion
	StaleMonths *int        `yaml:"stale_months,omitempty" json:"stale_months,omitempty" jsonschema:"default=24,description=Number of months to consider stale - 0 disables the check"` // Months since last update to consider stale
	Badges      []string    `yaml:"badges,omitempty" json:"badges,omitempty" jsonschema:"description=Array of badge names"`                                                             // List of badge names to display
	Categories  []*Category `yaml:"categories,omitempty" json:"categories,omitempty" jsonschema:"description=Array of categories"`                                                      // List categories; usually one auto category "Repositories"

	GroupByTopic      bool               `yaml:"group_by_topic" json:"group_by_topic,omitempty" jsonschema:"default=false,description=Group repositories by GitHub topic (default: false)"`                    // Group repositories by GitHub topic (default: false)
	TopicFallback     string             `yaml:"topic_fallback" json:"topic_fallback,omitempty" jsonschema:"default=misc,description=Fallback topic name if none found (default: misc)"`                       // Fallback topic name if none found (default: "misc")
	TopicGroupingMode *TopicGroupingMode `yaml:"topic_grouping_mode" json:"topic_grouping_mode,omitempty" jsonschema:"default=flat,description=Topic grouping style - flat or nested (default: flat)"`         // Topic grouping style: "flat" or "nested" (default: "flat")
	StarsFormat       *StarsFormat       `yaml:"stars_format" json:"stars_format,omitempty" jsonschema:"default=locale,description=Format for displaying stars - locale or compact or none (default: locale)"` // Format for displaying stars: "locale", "compact", "none" (default: "locale")
	Locale            string             `yaml:"locale" json:"locale,omitempty" jsonschema:"description=Locale for formatting numbers - BCP-47 (default: en-GB)"`                                              // Locale for formatting numbers, BCP-47 (default: "en-GB")

	TopTagsLimit *int              `yaml:"top_tags_limit,omitempty" json:"top_tags_limit,omitempty" jsonschema:"default=10,description=Maximum number of top tags to group by - 0 means no limit"` // Maximum number of top tags to group by
	MinTagCount  *int              `yaml:"min_tag_count,omitempty" json:"min_tag_count,omitempty" jsonschema:"default=1,description=Minimum count for a tag to be considered a top tag"`           // Minimum count for a tag to be considered a top tag
	TagAliases   map[string]string `yaml:"tag_aliases,omitempty" json:"tag_aliases,omitempty" jsonschema:"description=Map of tag aliases for normalization"`                                       // Map of tag aliases for normalization
}

// Default values for the optional list settings. A pointer field is only
// defaulted when it was left unset, so an explicit 0 keeps its meaning.
const (
	DefaultStaleMonths  = 24
	DefaultTopTagsLimit = 10
	DefaultMinTagCount  = 1
)

// IntOrDefault returns the value p points at, or def when p is nil.
func IntOrDefault(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// StaleMonthsValue returns the staleness window in months. Zero means the
// check is disabled.
func (l List) StaleMonthsValue() int {
	return IntOrDefault(l.StaleMonths, DefaultStaleMonths)
}

// TopTagsLimitValue returns the maximum number of topic groups. Zero means no
// limit.
func (l List) TopTagsLimitValue() int {
	return IntOrDefault(l.TopTagsLimit, DefaultTopTagsLimit)
}

// MinTagCountValue returns the minimum number of repositories a topic must
// appear in before it becomes a group.
func (l List) MinTagCountValue() int {
	return IntOrDefault(l.MinTagCount, DefaultMinTagCount)
}

type StarsFormat string

const (
	StarsFormatLocale  StarsFormat = "locale"
	StarsFormatCompact StarsFormat = "compact"
	StarsFormatNone    StarsFormat = "none"
)

type TopicGroupingMode string

const (
	TopicGroupingModeFlat   TopicGroupingMode = "flat"
	TopicGroupingModeNested TopicGroupingMode = "nested"
)

type Source struct {
	User string `yaml:"user" json:"user" jsonschema:"required,description=GitHub username or organization"` // GitHub username or organization
	Slug string `yaml:"slug" json:"slug" jsonschema:"required,description=Repository name or slug"`         // Repository name or slug
}

type Category struct {
	Name  string  `yaml:"name" json:"name" jsonschema:"required,description=Category name"`
	Items []*Item `yaml:"items" json:"items,omitempty" jsonschema:"description=Array of items in this category - Manually set if required"`
}

type ItemType string

// ItemType represents the type of item (GitHub repo or external link)
const (
	ItemGitHub ItemType = "github" // GitHub repository item
	ItemLink   ItemType = "link"   // External link item
)

type Item struct {
	Type   ItemType         `yaml:"type,omitempty" json:"type,omitempty" jsonschema:"description=Item type - either github repository or link - inferred from repo or url when omitted"` // Type of item: "github" or "link"
	Repo   string           `yaml:"repo,omitempty" json:"repo,omitempty" jsonschema:"description=Repository in owner/repo format (for github items)"`                                    // GitHub owner/repo (for GitHub items)
	URL    string           `yaml:"url,omitempty" json:"url,omitempty" jsonschema:"description=URL (for link items)"`                                                                    // External URL (for link items)
	Title  string           `yaml:"title,omitempty" json:"title,omitempty" jsonschema:"description=Item title"`                                                                          // Display title for the item
	Notes  string           `yaml:"notes,omitempty" json:"notes,omitempty" jsonschema:"description=Additional notes"`                                                                    // Additional notes or description
	GHMeta *github.RepoMeta `yaml:"-" json:"-"`                                                                                                                                          // GitHub metadata (populated at runtime)
}

// IsGitHub reports whether the item refers to a GitHub repository. An item
// without an explicit type is a repository when it names one or when
// metadata has already been fetched for it.
func (i *Item) IsGitHub() bool {
	if i == nil {
		return false
	}
	switch i.Type {
	case ItemGitHub:
		return true
	case ItemLink:
		return false
	}
	return i.Repo != "" || i.GHMeta != nil
}

// IsLink reports whether the item refers to an external link.
func (i *Item) IsLink() bool {
	if i == nil {
		return false
	}
	switch i.Type {
	case ItemLink:
		return true
	case ItemGitHub:
		return false
	}
	return i.Repo == "" && i.GHMeta == nil && i.URL != ""
}

// SeedRepos converts a flat list of owner/repo strings into one Category named "Repositories".
//
// Manually configured categories are preserved: the scraped repositories are
// added as their own category rather than replacing the first one, so
// hand-curated entries are never silently discarded.
func (l *List) SeedRepos(slugs []string) {
	items := make([]*Item, 0, len(slugs))
	for _, r := range slugs {
		items = append(items, &Item{Type: ItemGitHub, Repo: r})
	}
	if len(l.Categories) == 0 {
		l.Categories = []*Category{{Name: "Repositories", Items: items}}
		return
	}
	for _, c := range l.Categories {
		if c != nil && c.Name == "Repositories" {
			c.Items = append(c.Items, items...)
			return
		}
	}
	l.Categories = append(l.Categories, &Category{Name: "Repositories", Items: items})
}
