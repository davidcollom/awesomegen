package render

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/davidcollom/awesomegen/internal/config"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func humanStars(n int, format, locale string) string {
	switch format {
	case "none":
		return ""
	case "compact":
		switch {
		case n >= 1_000_000:
			return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
		case n >= 10_000:
			return fmt.Sprintf("%dk", n/1_000) // no decimals for ≥10k
		case n >= 1_000:
			return fmt.Sprintf("%.1fk", float64(n)/1_000)
		default:
			return strconv.Itoa(n)
		}
	default: // "locale"
		tag := language.Make(locale)
		p := message.NewPrinter(tag)
		return p.Sprintf("%d", n)
	}
}

func sortItems(items []config.Item) {
	slices.SortFunc(items, func(a, b config.Item) int {
		typeRank := func(t config.ItemType) int {
			if t == config.ItemGitHub {
				return 0
			}
			return 1
		}
		if r := typeRank(a.Type) - typeRank(b.Type); r != 0 {
			return r
		}
		ak := key(a)
		bk := key(b)
		switch {
		case ak < bk:
			return -1
		case ak > bk:
			return 1
		default:
			return 0
		}
	})
}

func key(i config.Item) string {
	if i.Type == config.ItemGitHub && i.GHMeta != nil {
		return i.GHMeta.FullName
	}
	if i.Title != "" {
		return i.Title
	}
	return i.URL
}

var splitTokens = regexp.MustCompile(`([A-Za-z0-9]+|[^A-Za-z0-9]+)`)

func smartTitleCase(s, locale string) string {
	c := cases.Title(language.Make(locale))
	tokens := splitTokens.FindAllString(s, -1)
	for i, t := range tokens {
		if isAlnum(t) {
			tokens[i] = c.String(strings.ToLower(t)) // “json” -> “Json” (we fix JSON via overrides above)
		}
	}
	return strings.Join(tokens, "")
}

func isAlnum(s string) bool {
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			continue
		}
		return false
	}
	return len(s) > 0
}

// Better slug for headings that tolerates '/', '_', '.'
var nonSlug = regexp.MustCompile(`[^a-z0-9\-]+`)
var dashes = regexp.MustCompile(`-+`)

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "-", "/", "-", "_", "-", ".", "-").Replace(s)
	s = nonSlug.ReplaceAllString(s, "-")
	s = dashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// Display a tag nicely for headings/TOC.
// 1) explicit overrides (from config), 2) built-in overrides,
// 3) smart title-case that respects separators and keeps digits.
func displayTag(tag, locale string, overrides map[string]string) string {
	if tag == "" {
		return tag
	}
	raw := strings.ToLower(tag)

	if overrides != nil {
		if v, ok := overrides[raw]; ok && v != "" {
			return v
		}
	}
	if v, ok := defaultTagDisplays[raw]; ok {
		return v
	}
	// fallback: smart Title Case
	return smartTitleCase(tag, locale)
}
