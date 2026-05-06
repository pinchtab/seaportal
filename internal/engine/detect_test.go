package engine

import "testing"

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
