package render_test

import (
	"testing"

	"github.com/davidcollom/awesomegen/internal/render"
	"github.com/stretchr/testify/require"
)

func TestTitlecaseKeys_English(t *testing.T) {
	tests := []struct {
		input  string
		locale string
		want   string
	}{
		{"hello world", "en", "Hello World"},
		{"multiple words here", "en", "Multiple Words Here"},
		{"already Titlecased", "en", "Already Titlecased"},
		{"", "en", ""},
		{"123 abc", "en", "123 Abc"},
	}

	for _, tt := range tests {
		got := render.TitlecaseKeys(tt.input, tt.locale)
		require.Equal(t, tt.want, got)
	}
}

func TestTitlecaseKeys_TurkishLocale(t *testing.T) {
	// Turkish locale has special casing rules for 'i'
	input := "istanbul izmir"
	locale := "tr"
	want := "İstanbul İzmir" // Turkish dotted capital I

	got := render.TitlecaseKeys(input, locale)
	require.Equal(t, want, got)
}

func TestTitlecaseKeys_NonAlpha(t *testing.T) {
	input := "foo-bar baz_qux"
	locale := "en"
	want := "Foo-Bar Baz_qux"

	got := render.TitlecaseKeys(input, locale)
	require.Equal(t, want, got)
}

// TestTitlecaseKeysKeepsTheCapitalsOfAnAcronym makes sure that a word that
// is written in capitals keeps them.
//
// The topics of GitHub are written that way - "CLI", "LLM", "AWS" - and the
// heading of a section has to read like the topic that was curated. A
// heading that reads "Cli" is not it.
func TestTitlecaseKeysKeepsTheCapitalsOfAnAcronym(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
	}{
		{"CLI tools", "CLI Tools"},
		{"LLM and agents", "LLM And Agents"},
		{"aws sdk", "Aws Sdk"},
		{"mySQL", "MySQL"},
	} {
		require.Equal(t, tt.want, render.TitlecaseKeys(tt.input, "en"))
	}
}
