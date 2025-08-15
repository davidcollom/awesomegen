package render

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/davidcollom/awesomegen/internal/config"
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

func slug(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"))
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
