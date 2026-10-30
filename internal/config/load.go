package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
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
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
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
		if strings.TrimSpace(s.Slug) == "" {
			return fmt.Errorf("lists[%d].sources[%d].slug: is required", idx, j)
		}
	}
	if l.Title == "" {
		return fmt.Errorf("lists[%d].title: is required", idx)
	}
	if l.MinStars < 0 {
		return fmt.Errorf("lists[%d].min_stars: must not be negative", idx)
	}
	if l.StaleMonths < 0 {
		return fmt.Errorf("lists[%d].stale_months: must not be negative", idx)
	}
	if l.TopTagsLimit < 0 {
		return fmt.Errorf("lists[%d].top_tags_limit: must not be negative", idx)
	}
	if l.MinTagCount < 0 {
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
		if err := validateOutputPath(l.Output); err != nil {
			return fmt.Errorf("lists[%d].output: %w", idx, err)
		}
	}
	return nil
}

// validateOutputPath ensures the output path stays inside the directory that
// holds the configuration file and cannot target hidden or executable files.
func validateOutputPath(out string) error {
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
		if l.StaleMonths == 0 {
			l.StaleMonths = 24
		}
		if l.TopTagsLimit == 0 {
			l.TopTagsLimit = 10
		}
		if l.MinTagCount == 0 {
			l.MinTagCount = 1
		}
	}
	return nil
}
