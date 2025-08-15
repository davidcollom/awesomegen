package render

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/davidcollom/awesomegen/internal/config"
)

func canonicalTag(raw string, aliases map[string]string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "" {
		return ""
	}
	if v, ok := aliases[t]; ok {
		return strings.ToLower(strings.TrimSpace(v))
	}
	return t
}

func rankTopTags(items []config.Item, aliases map[string]string, minCount, limit int) []string {
	counts := map[string]int{}
	for _, it := range items {
		if it.GHMeta == nil {
			continue
		}
		seen := map[string]struct{}{}
		for _, raw := range it.GHMeta.Topics {
			t := canonicalTag(raw, aliases)
			if t == "" {
				continue
			}
			if _, dup := seen[t]; dup {
				continue
			} // per-repo unique
			seen[t] = struct{}{}
			counts[t]++
		}
	}
	// filter by minCount
	for k, n := range counts {
		if n < minCount {
			delete(counts, k)
		}
	}
	// Collect iterator -> []string, then sort by freq desc, then alpha
	tags := slices.Collect(maps.Keys(counts))
	slices.SortFunc(tags, func(a, b string) int {
		if counts[a] != counts[b] {
			return cmp.Compare(counts[b], counts[a]) // desc by frequency
		}
		return cmp.Compare(a, b) // alpha
	})

	if limit > 0 && len(tags) > limit {
		tags = tags[:limit]
	}
	return tags
}
