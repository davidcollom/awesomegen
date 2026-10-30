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

// TestAnchorSetReservesGeneratedAnchors makes sure that an anchor that was
// generated for a repeated heading cannot be claimed by a later heading that
// happens to have the same name.
func TestAnchorSetReservesGeneratedAnchors(t *testing.T) {
	set := newAnchorSet()
	require.Equal(t, "ai", set.unique("AI"))
	require.Equal(t, "ai-1", set.unique("AI!"))
	// "ai-1" was already generated for the second "AI" heading, so this
	// heading has to be numbered further.
	require.Equal(t, "ai-1-1", set.unique("ai-1"))
	require.Equal(t, "ai-2", set.unique("AI"))
}
