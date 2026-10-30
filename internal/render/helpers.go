package render

import (
	"fmt"
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
		case n >= 100_000:
			return fmt.Sprintf("%dk", n/1_000) // no decimals for six figures
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

// htmlTagPattern matches an HTML tag. GitHub strips raw HTML from rendered
// Markdown, but third-party Markdown renderers (docsify, mkdocs, custom
// sites) frequently do not, so tags are removed before the text ever reaches
// the generated file.
//
// Only a tag that looks like one is removed: a name, optionally followed by
// attributes and an optional closing slash. Text that merely sits between
// angle brackets - "use x < y > z", an address in angle brackets - is prose
// and is kept, because dropping it would silently lose content.
var htmlTagPattern = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9-]*(?:\s[^<>]*)?/?>`)

// markdownEscaper escapes the characters that would otherwise be interpreted
// as Markdown syntax. It is applied to text controlled by third parties —
// repository descriptions, licenses and topic names — so that a repository
// owner cannot inject links, images or extra structure into the generated
// README.
//
// The ampersand is escaped as well: without it an HTML entity in the text
// would be resolved by the renderer, so the heading a reader sees would no
// longer be the text the anchor was derived from. Escaping it keeps the two
// the same.
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
	`&`, `\&`,
	`~`, `\~`,
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
//
// The angle brackets are encoded as well: a URL that carries them would
// otherwise end the link destination and turn the rest of it into raw HTML.
//
// The backslash is escaped as well: in Markdown it starts an escape
// sequence, so a URL that ends with one would swallow the bracket that
// closes the link and the rest of the line.
//
// The rules are the ones the configuration loader enforces, so that an entry
// that passed the validation is written and an entry that did not is
// reported while the configuration is read.
func sanitizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if err := config.ValidateLinkURL(raw); err != nil {
		return ""
	}
	return strings.NewReplacer(
		"(", "%28",
		")", "%29",
		" ", "%20",
		"<", "%3C",
		">", "%3E",
		"\\", "%5C",
	).Replace(raw)
}

// anchorStripper removes the characters GitHub removes when it derives a
// heading anchor.
//
// It mirrors the punctuation filter of the table of contents filter of
// GitHub's own HTML pipeline (github/html-pipeline,
// lib/html/pipeline/toc_filter.rb), which removes everything that is
// neither a word character, a hyphen nor a space. A word character is a
// letter, a mark, a number or a connector punctuation, so an underscore and
// the rare connector characters survive while an em dash or a curly quote do
// not.
var anchorStripper = regexp.MustCompile(`[^\p{L}\p{N}\p{M}\p{Pc} -]+`)

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

// anchorSet assigns the anchors to headings, mirroring the way GitHub
// de-duplicates repeated headings by appending a counter.
//
// The anchors are handed out exactly as GitHub hands them out: a heading
// that repeats the anchor of an earlier one gets the counter appended, and
// the counter is kept per anchor. A generated anchor is not reserved, so a
// heading whose own anchor is "ai-1" keeps it - GitHub hands out the same
// anchor twice in that case, and a link to it is at least a link that
// resolves.
type anchorSet struct {
	// seen counts how often a base anchor has been requested.
	seen map[string]int
}

func newAnchorSet() *anchorSet {
	return &anchorSet{
		seen: map[string]int{},
	}
}

// unique returns the anchor for heading, adding a numeric suffix if that
// anchor has already been used.
//
// GitHub counts the repetitions of a heading and appends the counter to the
// base anchor.
func (a *anchorSet) unique(heading string) string {
	base := Anchor(heading)
	n := a.seen[base]
	candidate := base
	if n > 0 {
		candidate = base + "-" + strconv.Itoa(n)
	}
	a.seen[base] = n + 1
	return candidate
}

// sortItems orders GitHub repositories before external links.
//
// The sort is stable: the curation order of the entries is preserved within
// each group, because that order is the only signal a curated list carries.
func sortItems(items []*config.Item) {
	slices.SortStableFunc(items, func(a, b *config.Item) int {
		typeRank := func(t *config.Item) int {
			if t.IsGitHub() {
				return 0
			}
			return 1
		}
		return typeRank(a) - typeRank(b)
	})
}
