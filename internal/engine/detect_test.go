package engine

import "testing"

// TestExtractMarkdownTitle covers the regression where the parser used
// strings.Index("---") to find the end of frontmatter and so stopped at the
// first "---" anywhere in the body — including inside a quoted YAML value.
func TestExtractMarkdownTitle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			// User's repro: a title containing "---" must not truncate the block.
			name:    "title containing --- substring",
			content: "---\ntitle: \"Hello --- world\"\n---\n# Fallback\n",
			want:    "Hello --- world",
		},
		{
			name:    "plain frontmatter title",
			content: "---\ntitle: Hello\n---\n# Body\n",
			want:    "Hello",
		},
		{
			name:    "single-quoted title",
			content: "---\ntitle: 'Hello world'\n---\n",
			want:    "Hello world",
		},
		{
			name:    "no frontmatter falls back to first heading",
			content: "# First Heading\n\nbody text\n",
			want:    "First Heading",
		},
		{
			name:    "frontmatter without title falls back to heading",
			content: "---\nauthor: ada\ndate: 2024-01-01\n---\n# Heading Wins\n",
			want:    "Heading Wins",
		},
		{
			// Unclosed frontmatter must not be parsed — fall back to heading.
			name:    "unclosed frontmatter falls back to heading",
			content: "---\ntitle: Ignored\n# Real Title\nstill in pseudo-frontmatter\n",
			want:    "Real Title",
		},
		{
			name:    "CRLF line endings",
			content: "---\r\ntitle: \"Hello --- world\"\r\n---\r\n# Heading\r\n",
			want:    "Hello --- world",
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name:    "frontmatter with empty title falls back to heading",
			content: "---\ntitle: \"\"\n---\n# Real\n",
			want:    "Real",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMarkdownTitle(tt.content)
			if got != tt.want {
				t.Errorf("extractMarkdownTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestExtractLLMsTxtURL covers the regression where the parser claimed to
// prefer `llms-full-txt` but actually returned the first matching entry,
// so a header listing `llms-txt` first yielded the smaller doc.
func TestExtractLLMsTxtURL(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "prefers full-txt when listed second",
			header: `</llms.txt>; rel="llms-txt", </llms-full.txt>; rel="llms-full-txt"`,
			want:   "/llms-full.txt",
		},
		{
			name:   "prefers full-txt when listed first",
			header: `</llms-full.txt>; rel="llms-full-txt", </llms.txt>; rel="llms-txt"`,
			want:   "/llms-full.txt",
		},
		{
			name:   "falls back to llms-txt when only it is present",
			header: `</llms.txt>; rel="llms-txt"`,
			want:   "/llms.txt",
		},
		{
			name:   "returns full-txt when only it is present",
			header: `</llms-full.txt>; rel="llms-full-txt"`,
			want:   "/llms-full.txt",
		},
		{
			name:   "ignores unrelated rels",
			header: `<https://example.com/style.css>; rel="stylesheet", </favicon.ico>; rel="icon"`,
			want:   "",
		},
		{
			name:   "empty header returns empty",
			header: ``,
			want:   "",
		},
		{
			name:   "unquoted rel value is accepted",
			header: `</llms-full.txt>; rel=llms-full-txt`,
			want:   "/llms-full.txt",
		},
		{
			// Substring-trap: URL contains the literal "llms-full-txt" but the
			// rel is "llms-txt". Must not be treated as the preferred entry.
			name:   "URL containing 'llms-full-txt' literal but rel=llms-txt",
			header: `</docs/llms-full-txt-spec>; rel="llms-txt"`,
			want:   "/docs/llms-full-txt-spec",
		},
		{
			name:   "case-insensitive rel attribute name",
			header: `</llms-full.txt>; REL="llms-full-txt"`,
			want:   "/llms-full.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLLMsTxtURL(tt.header)
			if got != tt.want {
				t.Errorf("extractLLMsTxtURL(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}
