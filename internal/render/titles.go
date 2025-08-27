package render

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// TitlecaseKeys takes a string title and a locale identifier, splits the title into words,
// and returns a new string where each word is converted to title case according to the specified locale.
// It uses the golang.org/x/text/cases and golang.org/x/text/language packages for locale-aware casing.
func TitlecaseKeys(title, locale string) string {
	words := strings.Fields(title)
	caser := cases.Title(language.Make(locale))
	for i, word := range words {
		words[i] = caser.String(word)
	}
	return strings.Join(words, " ")
}
