package render

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeTextRemovesMarkdownSyntax(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "a normal description", "a normal description"},
		{"link injection", "click [here](https://evil.example/pwn)", `click \[here\]\(https://evil.example/pwn\)`},
		{"image injection", "![x](https://evil.example/track.gif)", `\!\[x\]\(https://evil.example/track.gif\)`},
		{"html tag", "<img src=x onerror=alert(1)>", ""},
		{"script tag", "<script>alert(1)</script>", `alert\(1\)`},
		{"newline injection", "first\n- [evil](https://evil.example)", `first - \[evil\]\(https://evil.example\)`},
		{"emphasis", "some *bold* text", `some \*bold\* text`},
		{"underscore", "some _italic_ text", `some \_italic\_ text`},
		{"backtick", "some `code`", "some \\`code\\`"},
		{"heading", "# not a heading", `\# not a heading`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, sanitizeText(tt.in))
		})
	}
}

func TestSanitizeURL(t *testing.T) {
	require.Equal(t, "https://github.com/a/b", sanitizeURL("https://github.com/a/b"))
	require.Equal(t, "https://example.com/a%28b%29", sanitizeURL("https://example.com/a(b)"))
	require.Equal(t, "", sanitizeURL("javascript:alert(1)"))
	require.Equal(t, "", sanitizeURL("data:text/html;base64,PHNjcmlwdD4="))
	require.Equal(t, "", sanitizeURL(""))
	require.Equal(t, "", sanitizeURL("not a url"))
	// A URL without a scheme or without a host cannot be written.
	require.Equal(t, "", sanitizeURL("example.com/docs"))
	require.Equal(t, "", sanitizeURL("https://"))
}

// TestSanitizeURLEncodesAngleBrackets makes sure that a URL cannot end the
// link destination and turn the rest of it into raw HTML.
func TestSanitizeURLEncodesAngleBrackets(t *testing.T) {
	require.Equal(t, "https://example.com/%3Cscript%3E", sanitizeURL("https://example.com/<script>"))
	require.Equal(t, "https://example.com/a%3Cb", sanitizeURL("https://example.com/a<b"))
}

// TestSanitizeTextKeepsProseBetweenAngleBrackets makes sure that text which
// only happens to sit between angle brackets survives.
//
// Only a tag is removed: dropping the text would lose content.
func TestSanitizeTextKeepsProseBetweenAngleBrackets(t *testing.T) {
	require.Equal(t, `Use x \< y \> z`, sanitizeText("Use x < y > z"))
	require.Equal(t, `\<3`, sanitizeText("<3"))
	// A tag is still removed.
	require.Equal(t, "", sanitizeText("<b>"))
	require.Equal(t, "bold", sanitizeText("<b>bold</b>"))
}

// TestSanitizeTextEscapesStrikethrough makes sure that a description cannot
// strike through its own text.
//
// The tildes are the syntax of the strikethrough of GitHub flavoured
// Markdown.
func TestSanitizeTextEscapesStrikethrough(t *testing.T) {
	require.Equal(t, `\~\~deprecated\~\~`, sanitizeText("~~deprecated~~"))
}

func TestAnchorMatchesGitHub(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Ai", "ai"},
		{"Machine Learning", "machine-learning"},
		{"node.js", "nodejs"},
		{"c++", "c"},
		{"others & more", "others--more"},
		{"no  topics", "no--topics"},
		{"CI/CD", "cicd"},
		{"  Trimmed  ", "trimmed"},
		{"", ""},
		// GitHub keeps leading and trailing hyphens: it only removes
		// punctuation and replaces whitespace.
		{"Node-", "node-"},
		{"-Node", "-node"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, Anchor(tt.in), tt.in)
	}
}

func TestAnchorSetDeduplicates(t *testing.T) {
	set := newAnchorSet()
	require.Equal(t, "ai", set.unique("AI"))
	require.Equal(t, "ai-1", set.unique("ai"))
	require.Equal(t, "ai-2", set.unique("Ai"))
	require.Equal(t, "web", set.unique("Web"))
}

// TestAnchorSetFollowsGitHub makes sure that the anchors are handed out the
// way GitHub hands them out.
//
// GitHub counts the repetitions of a heading and appends the counter to the
// base anchor. It does not reserve the anchors it generates, so a heading
// whose own anchor is "ai-1" keeps that anchor, even when a second "AI"
// heading was given it before. A link to it resolves to the first heading
// with that anchor, which is what GitHub renders as well.
func TestAnchorSetFollowsGitHub(t *testing.T) {
	set := newAnchorSet()
	require.Equal(t, "ai", set.unique("AI"))
	require.Equal(t, "ai-1", set.unique("AI!"))
	require.Equal(t, "ai-1", set.unique("ai-1"))
	require.Equal(t, "ai-2", set.unique("AI"))
	require.Equal(t, "ai-1-1", set.unique("ai-1"))
}
