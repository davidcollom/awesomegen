package render

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// TitlecaseKeys takes a string title and a locale identifier, splits the title into words,
// and returns a new string where each word is converted to title case according to the specified locale.
// It uses the golang.org/x/text/cases and golang.org/x/text/language packages for locale-aware casing.
//
// The rest of a word is left as it is written: a word that is already in
// capitals keeps them. The topics of GitHub are written that way - "CLI",
// "LLM", "AWS" - and a heading that reads "Cli" is not the topic that was
// curated.
func TitlecaseKeys(title, locale string) string {
	words := strings.Fields(title)
	caser := cases.Title(language.Make(locale), cases.NoLower)
	for i, word := range words {
		words[i] = caser.String(word)
	}
	return strings.Join(words, " ")
}
