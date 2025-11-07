//go:generate go run generate_schema.go

package config

import (
	"github.com/davidcollom/awesomegen/internal/github"
	"github.com/invopop/jsonschema"

	// Used within generate_schema.go - but is ignored during normal builds
	_ "github.com/stoewer/go-strcase"
)

type Config struct {
	Version int    `yaml:"version"` // Configuration file version
	User    string `yaml:"user"`    // GitHub username or owner
	Lists   []List `yaml:"lists"`   // List of repository lists to generate
}

type List struct {
	// Unique identifier for the list
	Slug string `yaml:"slug" json:"slug" jsonschema:"required,description=List slug identifier"`
	// List of additional sources (users/repos) to include (not in the list)
	Sources     []*Source   `yaml:"sources,omitempty" jsonschema:"description=Array of additional sources to include"`
	Title       string      `yaml:"title" json:"title" jsonschema:"required,description=List title"`                                                  // Title of the list
	Tagline     string      `yaml:"tagline" json:"tagline" jsonschema:"required,description=List tagline"`                                            // Short description or tagline
	Output      string      `yaml:"output,omitempty" json:"output,omitempty" jsonschema:"description=Output path or format"`                          // Output file path or name
	MinStars    int         `yaml:"min_stars,omitempty" json:"min_stars,omitempty" jsonschema:"description=Minimum stars required to be included"`    // Minimum GitHub stars required for inclusion
	StaleMonths int         `yaml:"stale_months,omitempty" json:"stale_months,omitempty" jsonschema:"description=Number of months to consider stale"` // Months since last update to consider stale
	Badges      []string    `yaml:"badges,omitempty" json:"badges,omitempty" jsonschema:"description=Array of badge names"`                           // List of badge names to display
	Categories  []*Category `yaml:"categories,omitempty" json:"categories,omitempty" jsonschema:"description=Array of categories"`                    // List categories; usually one auto category "Repositories"

	GroupByTopic      bool               `yaml:"group_by_topic" jsonschema:"default=false,description=Group repositories by GitHub topic (default: false)"`                         // Group repositories by GitHub topic (default: false)
	TopicFallback     string             `yaml:"topic_fallback" jsonschema:"default=misc,description=Fallback topic name if none found (default: 'misc')"`                          // Fallback topic name if none found (default: "misc")
	TopicGroupingMode *TopicGroupingMode `yaml:"topic_grouping_mode" jsonschema:"default=flat,description=Topic grouping style: 'flat' or 'nested' (default: 'flat')"`              // Topic grouping style: "flat" or "nested" (default: "flat")
	StarsFormat       *StarsFormat       `yaml:"stars_format" jsonschema:"default=locale,description=Format for displaying stars: 'locale', 'compact', 'none' (default: 'locale')"` // Format for displaying stars: "locale", "compact", "none" (default: "locale")
	Locale            string             `yaml:"locale" jsonschema:"description=Locale for formatting numbers, BCP-47 (default: 'en-GB')"`                                          // Locale for formatting numbers, BCP-47 (default: "en-GB")

	GroupByTopTags bool              `yaml:"group_by_top_tags" jsonschema:"description=Group repositories by top tags"`                 // Group repositories by top tags
	TopTagsLimit   int               `yaml:"top_tags_limit" jsonschema:"description=Maximum number of top tags to group by"`            // Maximum number of top tags to group by
	MinTagCount    int               `yaml:"min_tag_count" jsonschema:"description=Minimum count for a tag to be considered a top tag"` // Minimum count for a tag to be considered a top tag
	SingleHome     bool              `yaml:"single_home" jsonschema:"description=If true, generate a single home page"`                 // If true, generate a single home page
	TagAliases     map[string]string `yaml:"tag_aliases" jsonschema:"optional,description=Map of tag aliases for normalization"`        // Map of tag aliases for normalization
}

type StarsFormat string

const (
	StarsFormatLocale  StarsFormat = "locale"
	StarsFormatCompact StarsFormat = "compact"
	StarsFormatNone    StarsFormat = "none"
)

// JSONSchema provides custom schema for StarsFormat
func (StarsFormat) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: []interface{}{StarsFormatLocale, StarsFormatCompact, StarsFormatNone},
	}
}

type TopicGroupingMode string

const (
	TopicGroupingModeFlat   TopicGroupingMode = "flat"
	TopicGroupingModeNested TopicGroupingMode = "nested"
)

// JSONSchema provides custom schema for TopicGroupingMode
func (TopicGroupingMode) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: []interface{}{TopicGroupingModeFlat, TopicGroupingModeNested},
	}
}

type Source struct {
	User string `yaml:"user"` // GitHub username or organization
	Slug string `yaml:"slug"` // Repository name or slug
}

type Category struct {
	Name    string  `yaml:"name" json:"name" jsonschema:"required,description=Category name"`
	Items   []*Item `yaml:"items" json:"items" jsonschema:"description=Array of items in this category - Manually set if required"`
	Version int     `yaml:"version" json:"version" jsonschema:"required,description=Configuration version"`
	User    string  `yaml:"user" json:"user" jsonschema:"required,description=User identifier"`
	Lists   []*List `yaml:"lists" json:"lists" jsonschema:"required,description=Array of lists"`
}

type ItemType string

// ItemType represents the type of item (GitHub repo or external link)
const (
	ItemGitHub ItemType = "github" // GitHub repository item
	ItemLink   ItemType = "link"   // External link item
)

// JSONSchema provides custom schema for ItemType
func (ItemType) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: []interface{}{ItemGitHub, ItemLink},
	}
}

type Item struct {
	Type   ItemType         `yaml:"type" json:"type" jsonschema:"required,description=Item type - either github repository or link"`                  // Type of item: "github" or "link"
	Repo   string           `yaml:"repo,omitempty" json:"repo,omitempty" jsonschema:"description=Repository in owner/repo format (for github items)"` // GitHub owner/repo (for GitHub items)
	URL    string           `yaml:"url,omitempty" json:"url,omitempty" jsonschema:"description=URL (for link items)"`                                 // External URL (for link items)
	Title  string           `yaml:"title,omitempty" json:"title,omitempty" jsonschema:"description=Item title"`                                       // Display title for the item
	Notes  string           `yaml:"notes,omitempty" json:"notes,omitempty" jsonschema:"description=Additional notes"`                                 // Additional notes or description
	GHMeta *github.RepoMeta `yaml:"-" json:"-"`                                                                                                       // GitHub metadata (populated at runtime)
}

// SeedRepos converts a flat list of owner/repo strings into one Category named "Repositories".
func (l *List) SeedRepos(slugs []string) {
	items := make([]*Item, 0, len(slugs))
	for _, r := range slugs {
		items = append(items, &Item{Type: ItemGitHub, Repo: r})
	}
	if len(l.Categories) == 0 {
		l.Categories = []*Category{{Name: "Repositories", Items: items}}
		return
	}
	// replace first category if exists
	l.Categories[0].Name = "Repositories"
	l.Categories[0].Items = items
}
