package render

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/davidcollom/awesomegen/internal/config"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func humanStars(n int, format config.StarsFormat, locale string) string {
	switch format {
	case config.StarsFormatNone:
		return ""
	case config.StarsFormatCompact:
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

// slug returns a stable, lowercase identifier for a heading key.
func slug(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"))
}

// htmlTagPattern matches anything that looks like an HTML tag. GitHub strips
// raw HTML from rendered Markdown, but third-party Markdown renderers
// (docsify, mkdocs, custom sites) frequently do not, so tags are removed
// before the text ever reaches the generated file.
var htmlTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)

// markdownEscaper escapes the characters that would otherwise be interpreted
// as Markdown syntax. It is applied to text controlled by third parties —
// repository descriptions, licenses and topic names — so that a repository
// owner cannot inject links, images or extra structure into the generated
// README.
//
// strings.NewReplacer performs a single pass, so the replacement text is
// never rescanned.
var markdownEscaper = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	`*`, `\*`,
	`_`, `\_`,
	`[`, `\[`,
	`]`, `\]`,
	`(`, `\(`,
	`)`, `\)`,
	`#`, `\#`,
	`!`, `\!`,
	`|`, `\|`,
	`<`, `\<`,
	`>`, `\>`,
)

// plainText removes HTML tags and collapses whitespace so that a
// description cannot break out of its list item. Unlike sanitizeText it
// leaves Markdown metacharacters alone: the result is the text a reader
// sees, and therefore the text GitHub derives a heading anchor from.
func plainText(s string) string {
	s = htmlTagPattern.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}

// sanitizeText makes untrusted text safe to embed in generated Markdown.
// It removes HTML tags, collapses whitespace (so a description cannot break
// out of its list item) and escapes Markdown metacharacters.
func sanitizeText(s string) string {
	return markdownEscaper.Replace(plainText(s))
}

// sanitizeURL validates that a URL is an absolute http/https URL and escapes
// the characters that would otherwise terminate a Markdown link.
func sanitizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return ""
	}
	return strings.NewReplacer("(", "%28", ")", "%29", " ", "%20").Replace(raw)
}

// anchorStripper removes the characters GitHub removes when it derives a
// heading anchor. This mirrors github-slugger: letters, numbers, marks,
// spaces, underscores and hyphens survive; everything else is dropped.
var anchorStripper = regexp.MustCompile(`[^\p{L}\p{N}\p{M} _-]+`)

// Anchor converts a heading into the anchor GitHub generates for it.
//
// Leading and trailing hyphens are part of the anchor: GitHub only removes
// punctuation and replaces whitespace, so the heading "Node-" has the anchor
// "node-" and not "node".
func Anchor(heading string) string {
	s := strings.ToLower(strings.TrimSpace(heading))
	s = anchorStripper.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

// anchorSet assigns unique anchors to headings, mirroring the way GitHub
// de-duplicates repeated headings by appending a counter.
//
// The anchors it hands out are reserved, so a heading can never claim an
// anchor that was already generated for a different heading.
type anchorSet struct {
	// seen counts how often a base anchor has been requested.
	seen map[string]int
	// used holds every anchor that has been handed out.
	used map[string]struct{}
}

func newAnchorSet() *anchorSet {
	return &anchorSet{
		seen: map[string]int{},
		used: map[string]struct{}{},
	}
}

// unique returns the anchor for heading, adding a numeric suffix if that
// anchor has already been used.
//
// GitHub counts the repetitions of a heading and appends the counter to the
// base anchor. A generated anchor is reserved as well, so a later heading
// whose own anchor is "ai-1" does not take the "ai-1" that was already
// generated for a second "ai" heading.
func (a *anchorSet) unique(heading string) string {
	base := Anchor(heading)
	n := a.seen[base]
	candidate := base
	if n > 0 {
		candidate = base + "-" + strconv.Itoa(n)
	}
	for {
		if _, taken := a.used[candidate]; !taken {
			break
		}
		n++
		candidate = base + "-" + strconv.Itoa(n)
	}
	a.seen[base] = n + 1
	a.used[candidate] = struct{}{}
	return candidate
}

func sortItems(items []*config.Item) {
	slices.SortFunc(items, func(a, b *config.Item) int {
		typeRank := func(t config.ItemType) int {
			if t == config.ItemGitHub {
				return 0
			}
			return 1
		}
		if r := typeRank(a.Type) - typeRank(b.Type); r != 0 {
			return r
		}
		ak := key(*a)
		bk := key(*b)
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
