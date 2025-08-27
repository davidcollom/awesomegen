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
