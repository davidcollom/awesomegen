package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"

	"github.com/davidcollom/awesomegen/internal/github"
)

// MaxConfigBytes bounds the size of a configuration file. The whole file is
// read into memory, so an unbounded read is a memory-exhaustion vector.
const MaxConfigBytes = 4 << 20 // 4 MiB

// ConfigVersion is the current configuration schema version.
const ConfigVersion = 1

// allowedOutputExtensions restricts the files awesomegen is allowed to
// write. Without this an `output` value could target a workflow file or a
// git hook inside the generated repository.
var allowedOutputExtensions = map[string]bool{
	".md":       true,
	".markdown": true,
	".txt":      true,
}

// Load reads, parses and validates a configuration file.
func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return Config{}, err
	}
	if fi.Size() > MaxConfigBytes {
		return Config{}, fmt.Errorf("config file %s is too large (%d bytes, limit is %d bytes)", path, fi.Size(), MaxConfigBytes)
	}

	b, err := io.ReadAll(io.LimitReader(f, MaxConfigBytes+1))
	if err != nil {
		return Config{}, err
	}
	if len(b) > MaxConfigBytes {
		return Config{}, fmt.Errorf("config file %s is too large (limit is %d bytes)", path, MaxConfigBytes)
	}

	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	// Reject unknown fields so a misspelled key is reported instead of
	// being silently ignored.
	dec.KnownFields(true)
	// A file that holds no document at all - an empty file, or one that
	// carries nothing but comments - leaves the configuration empty, and
	// the validation below reports what is missing. That is more helpful
	// than the "EOF" of the decoder.
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Consume the entire stream: trailing empty documents carry no settings,
	// but must not hide a later document with content or invalid YAML.
	for {
		var extra yaml.Node
		if err := dec.Decode(&extra); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", path, err)
		}
		if len(extra.Content) == 1 {
			node := extra.Content[0]
			if node.Kind == yaml.ScalarNode && node.Tag == "!!null" && node.Value == "" && node.Style == 0 {
				continue
			}
		}
		return Config{}, fmt.Errorf("parse config %s: the file must hold exactly one YAML document", path)
	}

	if err := c.validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config %s: %w", path, err)
	}
	if err := c.applyDefaults(); err != nil {
		return Config{}, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return c, nil
}

func (c *Config) validate() error {
	if c.Version != 0 && c.Version != ConfigVersion {
		return fmt.Errorf("unsupported config version %d (this build supports version %d)", c.Version, ConfigVersion)
	}
	if len(c.Lists) == 0 {
		return fmt.Errorf("lists is required")
	}
	// The top level user is the owner of a source when a list does not
	// name one itself, so it is held to the same rules as a source user.
	if c.User != "" && !github.ValidOwner(c.User) {
		return fmt.Errorf("user: %q is not a valid GitHub user or organisation name", c.User)
	}
	for i := range c.Lists {
		if err := c.Lists[i].validate(i, c.User); err != nil {
			return err
		}
	}
	return nil
}

func (l *List) validate(idx int, configUser string) error {
	if len(l.Sources) == 0 {
		// Legacy fallback requires a top-level user plus this list's slug.
		if l.Slug == "" {
			return fmt.Errorf("lists[%d]: either sources or {user + slug} must be set", idx)
		}
		if configUser == "" {
			return fmt.Errorf("lists[%d]: user is required when sources is not set", idx)
		}
	}
	for j, s := range l.Sources {
		if s == nil {
			return fmt.Errorf("lists[%d].sources[%d]: must not be null", idx, j)
		}
		if strings.TrimSpace(s.User) == "" {
			return fmt.Errorf("lists[%d].sources[%d].user: is required", idx, j)
		}
		// The user and the slug are used to build the URL of the Star
		// List. The check is the one the scraper applies, so a
		// configuration that names an impossible user is rejected here
		// and not when the first request is made.
		if !github.ValidOwner(s.User) {
			return fmt.Errorf("lists[%d].sources[%d].user: %q is not a valid GitHub user or organisation name", idx, j, s.User)
		}
		if strings.TrimSpace(s.Slug) == "" {
			return fmt.Errorf("lists[%d].sources[%d].slug: is required", idx, j)
		}
		if !github.ValidRepoName(s.Slug) {
			return fmt.Errorf("lists[%d].sources[%d].slug: %q is not a valid Star List slug", idx, j, s.Slug)
		}
	}
	if l.Title == "" {
		return fmt.Errorf("lists[%d].title: is required", idx)
	}
	// A title that holds nothing but spaces is rendered as an empty
	// heading, so the title is trimmed and rejected when nothing is left.
	trimmed := strings.TrimSpace(l.Title)
	if trimmed == "" {
		return fmt.Errorf("lists[%d].title: must not be blank", idx)
	}
	l.Title = trimmed
	if l.MinStars < 0 {
		return fmt.Errorf("lists[%d].min_stars: must not be negative", idx)
	}
	if l.StaleMonths != nil && *l.StaleMonths < 0 {
		return fmt.Errorf("lists[%d].stale_months: must not be negative", idx)
	}
	if l.TopTagsLimit != nil && *l.TopTagsLimit < 0 {
		return fmt.Errorf("lists[%d].top_tags_limit: must not be negative", idx)
	}
	if l.MinTagCount != nil && *l.MinTagCount < 0 {
		return fmt.Errorf("lists[%d].min_tag_count: must not be negative", idx)
	}
	if l.TopicGroupingMode != nil {
		switch *l.TopicGroupingMode {
		case TopicGroupingModeFlat, TopicGroupingModeNested:
		default:
			return fmt.Errorf("lists[%d].topic_grouping_mode: must be one of %q or %q", idx, TopicGroupingModeFlat, TopicGroupingModeNested)
		}
	}
	if l.StarsFormat != nil {
		switch *l.StarsFormat {
		case StarsFormatLocale, StarsFormatCompact, StarsFormatNone:
		default:
			return fmt.Errorf("lists[%d].stars_format: must be one of %q, %q or %q", idx, StarsFormatLocale, StarsFormatCompact, StarsFormatNone)
		}
	}
	if l.Locale != "" {
		if _, err := language.Parse(l.Locale); err != nil {
			return fmt.Errorf("lists[%d].locale: %q is not a valid BCP-47 tag", idx, l.Locale)
		}
	}
	if l.Output != "" {
		if err := ValidateOutputPath(l.Output); err != nil {
			return fmt.Errorf("lists[%d].output: %w", idx, err)
		}
	}
	// The badges are images, so they are URLs. A badge that is not a URL
	// would be dropped from the generated README without a word.
	for bi, b := range l.Badges {
		if err := ValidateLinkURL(b); err != nil {
			return fmt.Errorf("lists[%d].badges[%d]: %w", idx, bi, err)
		}
	}
	// The slug identifies the list and is used to derive a file name when no
	// explicit output is set, so it has to obey the same conservative
	// character set as a scraped star-list slug.
	if l.Slug != "" && !github.ValidRepoName(l.Slug) {
		return fmt.Errorf("lists[%d].slug: %q must contain only letters, digits, dots, dashes and underscores", idx, l.Slug)
	}
	for ci, c := range l.Categories {
		if c == nil {
			return fmt.Errorf("lists[%d].categories[%d]: must not be null", idx, ci)
		}
		for ii, it := range c.Items {
			if it == nil {
				return fmt.Errorf("lists[%d].categories[%d].items[%d]: must not be null", idx, ci, ii)
			}
			switch it.Type {
			case ItemGitHub:
				if strings.TrimSpace(it.Repo) == "" {
					return fmt.Errorf("lists[%d].categories[%d].items[%d].repo: is required for a github item", idx, ci, ii)
				}
				// A malformed repository name is rejected here: the
				// lookup would fail later and the entry would be
				// dropped from the list without a word.
				if _, _, err := github.SplitOwnerRepo(it.Repo); err != nil {
					return fmt.Errorf("lists[%d].categories[%d].items[%d].repo: %w", idx, ci, ii, err)
				}
			case ItemLink:
				if strings.TrimSpace(it.URL) == "" {
					return fmt.Errorf("lists[%d].categories[%d].items[%d].url: is required for a link item", idx, ci, ii)
				}
				if err := ValidateLinkURL(it.URL); err != nil {
					return fmt.Errorf("lists[%d].categories[%d].items[%d].url: %w", idx, ci, ii, err)
				}
			case "":
				// The type is inferred from repo or url when it is
				// omitted, so one of them has to be present. A repo
				// is validated here as well: the inference happens
				// after the validation, so an invalid repository
				// would otherwise only be detected when the lookup
				// failed, and the entry would be dropped from the
				// list without a word.
				//
				// The raw values are inspected and not the trimmed
				// ones: the inference uses the raw value as well, so
				// a repository of " " would turn the entry into a
				// lookup that always fails, and the entry would be
				// dropped from the list without a word.
				switch {
				case it.Repo != "":
					if _, _, err := github.SplitOwnerRepo(it.Repo); err != nil {
						return fmt.Errorf("lists[%d].categories[%d].items[%d].repo: %w", idx, ci, ii, err)
					}
				case strings.TrimSpace(it.URL) != "":
					if err := ValidateLinkURL(it.URL); err != nil {
						return fmt.Errorf("lists[%d].categories[%d].items[%d].url: %w", idx, ci, ii, err)
					}
				case it.URL != "":
					return fmt.Errorf("lists[%d].categories[%d].items[%d].url: must not be blank", idx, ci, ii)
				default:
					return fmt.Errorf("lists[%d].categories[%d].items[%d]: repo: or url: is required", idx, ci, ii)
				}
			default:
				return fmt.Errorf("lists[%d].categories[%d].items[%d].type: must be one of %q or %q", idx, ci, ii, ItemGitHub, ItemLink)
			}
		}
	}
	return nil
}

// ValidateLinkURL ensures that a URL can be written as a link.
//
// The renderer only writes an absolute http(s) URL that names a host, so a
// link that carries anything else - a missing scheme, a "mailto:" address, a
// "javascript:" address - would be dropped from the generated README without
// a word. The check lives here, so that the loader and the renderer agree
// on what a usable URL is.
func ValidateLinkURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%q is not a valid URL", raw)
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("%q is not an absolute http(s) URL", raw)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%q does not name a host", raw)
	}
	return nil
}

// ValidateOutputPath ensures an output path stays inside the directory that
// holds the configuration file and cannot target hidden or executable files.
// It is exported so that paths derived at runtime - such as the per-list
// filename generated from a list slug - are held to the same rules as
// explicitly configured output paths.
func ValidateOutputPath(out string) error {
	if filepath.IsAbs(out) {
		return fmt.Errorf("must be a relative path")
	}
	clean := filepath.Clean(out)
	if clean == "." || clean == "" {
		return fmt.Errorf("must name a file")
	}
	for _, seg := range strings.Split(filepath.ToSlash(clean), "/") {
		if seg == ".." {
			return fmt.Errorf("must not contain %q", "..")
		}
		// Hidden segments cover .git, .github and dotfiles.
		if strings.HasPrefix(seg, ".") {
			return fmt.Errorf("must not contain hidden path segments (%q)", seg)
		}
	}
	ext := strings.ToLower(filepath.Ext(clean))
	if !allowedOutputExtensions[ext] {
		return fmt.Errorf("must use one of the extensions %s", strings.Join(sortedExtensions(), ", "))
	}
	return nil
}

func sortedExtensions() []string {
	out := make([]string, 0, len(allowedOutputExtensions))
	for ext := range allowedOutputExtensions {
		out = append(out, ext)
	}
	// small, fixed set - keep the order stable
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// applyDefaults fills in the values that were not set by the user.
//
// The optional numeric settings are pointers, so a value the user set
// explicitly - including 0 - is never overwritten by a default.
func (c *Config) applyDefaults() error {
	for i := range c.Lists {
		l := &c.Lists[i]
		if len(l.Sources) == 0 {
			// legacy fallback requires user+slug
			l.Sources = []*Source{{User: c.User, Slug: l.Slug}}
		}
		if l.TopicFallback == "" {
			l.TopicFallback = "misc"
		}
		if l.TopicGroupingMode == nil {
			mode := TopicGroupingModeFlat
			l.TopicGroupingMode = &mode
		}
		if l.StarsFormat == nil {
			mode := StarsFormatLocale
			l.StarsFormat = &mode
		}
		if l.Locale == "" {
			l.Locale = "en-GB"
		}
		aliases, err := NormaliseTagAliases(l.TagAliases)
		if err != nil {
			return fmt.Errorf("lists[%d]: %w", i, err)
		}
		l.TagAliases = aliases
		l.inferItemTypes()
	}
	return nil
}

// NormaliseTagAliases returns a copy of an alias map whose names and targets
// are trimmed and lowercased.
//
// A topic is lowercased before it is looked up in the map, so an alias whose
// name is not lowercase could never match. The names are normalised here, so
// that the aliases are matched case-insensitively, as documented.
//
// The names are visited in sorted order, so that two names that normalise to
// the same alias always resolve to the same target and the error message of
// an ambiguous map is the same every time.
func NormaliseTagAliases(aliases map[string]string) (map[string]string, error) {
	if len(aliases) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make(map[string]string, len(aliases))
	origin := make(map[string]string, len(aliases))
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			return nil, fmt.Errorf("tag_aliases: the name of an alias must not be empty")
		}
		target := strings.ToLower(strings.TrimSpace(aliases[name]))
		if target == "" {
			return nil, fmt.Errorf("tag_aliases: the alias %q must not point at an empty tag", name)
		}
		if previous, ok := out[key]; ok {
			// Two names that only differ in case are the same alias.
			// That is only a problem when they point at different tags,
			// because then there is no answer to which one wins.
			if previous != target {
				return nil, fmt.Errorf("tag_aliases: %q and %q both name the alias %q, but they point at different tags", origin[key], name, key)
			}
			continue
		}
		out[key] = target
		origin[key] = name
	}
	return out, nil
}

// inferItemTypes fills in the type of items that do not declare one, so an
// item is never silently dropped for omitting an optional field.
func (l *List) inferItemTypes() {
	for _, c := range l.Categories {
		if c == nil {
			continue
		}
		for _, it := range c.Items {
			if it == nil || it.Type != "" {
				continue
			}
			switch {
			case it.Repo != "":
				it.Type = ItemGitHub
			case it.URL != "":
				it.Type = ItemLink
			}
		}
	}
}
