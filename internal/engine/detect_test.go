package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractMarkdownTitle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
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

func loadBlockedFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "blocked", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(data)
}

func TestDetectBlocked_AdditionalWAFs(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		fixture string
		want    bool
	}{
		// Positive: synthetic challenge fixtures.
		{name: "akamai challenge fixture", fixture: "challenge-akamai.html", want: true},
		{name: "datadome challenge fixture", fixture: "challenge-datadome.html", want: true},
		{name: "imperva modern challenge fixture", fixture: "challenge-imperva-modern.html", want: true},

		// Negative: marketing/docs/prose that mentions the WAFs without the challenge markers.
		{
			name: "akamai marketing page mentioning bot manager",
			html: `<!DOCTYPE html><html><head><title>Bot Manager | Akamai</title></head>` +
				`<body><h1>Akamai Bot Manager</h1>` +
				`<p>Akamai Bot Manager detects and mitigates bot traffic. Learn how Bot Manager defends against credential stuffing.</p>` +
				`<p>Our customers use Bot Manager across thousands of properties. Pure marketing prose, no challenge markers.</p>` +
				`<p>` + longFiller() + `</p></body></html>`,
			want: false,
		},
		{
			name: "datadome mentioned in a JS comment without challenge assets",
			html: `<!DOCTYPE html><html><head><title>Security FAQ</title></head>` +
				`<body><h1>Security FAQ</h1>` +
				`<p>We document our anti-abuse stack here for transparency.</p>` +
				`<script>// We use datadome for security on selected endpoints. Not a tag, just a note.</script>` +
				`<p>` + longFiller() + `</p></body></html>`,
			want: false,
		},
		{
			name: "tutorial mentioning request unsuccessful without full incapsula pattern",
			html: `<!DOCTYPE html><html><head><title>HTTP Error Tutorial</title></head>` +
				`<body><h1>When a request is unsuccessful</h1>` +
				`<p>A request unsuccessful response can be caused by many things: rate-limiting middleware, ` +
				`origin downtime, or aggressive WAFs. This tutorial walks through diagnosis steps.</p>` +
				`<p>` + longFiller() + `</p></body></html>`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := tt.html
			if tt.fixture != "" {
				html = loadBlockedFixture(t, tt.fixture)
			}
			got := DetectBlocked(html)
			if got != tt.want {
				t.Errorf("DetectBlocked() = %v, want %v", got, tt.want)
			}
		})
	}
}

// longFiller returns >1 KB of prose so negative cases skip the short-body
// blockedPatterns sweep and only rely on headPatterns precision.
func longFiller() string {
	const sentence = "This is filler prose intended to push the body length past the short-page threshold so the detector relies only on high-precision head patterns. "
	out := ""
	for i := 0; i < 12; i++ {
		out += sentence
	}
	return out
}

// regression: detect-spa-non-ascii-panic
func TestDetectSPA_HandlesNonASCIIBodyContent(t *testing.T) {
	// Turkish capital İ lowercases to two-byte "i̇" — ToLower would shift
	// byte offsets and the prior implementation panicked slicing the
	// original string with indexes from the lowered copy.
	html := `<html><head><title>İçindekiler</title></head><body><p>İstanbul ve İzmir hakkında.</p></body></html>`
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DetectSPA panicked on non-ASCII input: %v", r)
		}
	}()
	signals, _ := DetectSPA(html)
	// Body text is short; expect minimal-body-content signal.
	found := false
	for _, s := range signals {
		if s == "minimal-body-content" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected minimal-body-content signal for short non-ASCII body, got %v", signals)
	}
}

// regression: detect-spa-non-ascii-panic
func TestDetectSPA_HandlesEmptyBody(t *testing.T) {
	html := `<html><head></head><body></body></html>`
	signals, _ := DetectSPA(html)
	found := false
	for _, s := range signals {
		if s == "minimal-body-content" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected minimal-body-content signal for empty body, got %v", signals)
	}
}

// regression: detect-spa-non-ascii-panic
func TestDetectSPA_NoBodyTag(t *testing.T) {
	html := `<html><head><title>İstanbul</title></head></html>`
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DetectSPA panicked on missing body: %v", r)
		}
	}()
	signals, _ := DetectSPA(html)
	for _, s := range signals {
		if s == "minimal-body-content" {
			t.Errorf("did not expect minimal-body-content without body tag, got %v", signals)
		}
	}
}

// regression: classifier-round-2-fallback — DetectJSChallenge fires on
// small HTML bodies that ship CDN/anti-bot challenge signatures. The
// 1500-byte length cap is the key gatekeeper; large pages that merely
// mention "cloudflare" in a footer must NOT match.
func TestDetectJSChallenge_CloudflarePositive(t *testing.T) {
	html := `<html><head><title>Just a moment...</title></head><body>` +
		`<script>window._cf_chl_opt = {cType: 'managed'};</script>` +
		`<script src="/cdn-cgi/challenge-platform/h/b/orchestrate"></script>` +
		`</body></html>`
	if !DetectJSChallenge(html, "text/html; charset=UTF-8", len(html)) {
		t.Error("expected DetectJSChallenge=true on small Cloudflare challenge HTML")
	}
}

func TestDetectJSChallenge_DataDomePositive(t *testing.T) {
	html := `<html><body><script>var dd = {datadome: "x"}; ` +
		`var src = "https://dd.datadome.co/captcha";</script></body></html>`
	if !DetectJSChallenge(html, "text/html", len(html)) {
		t.Error("expected DetectJSChallenge=true on small DataDome challenge HTML")
	}
}

func TestDetectJSChallenge_LargeBodyNegative(t *testing.T) {
	// Pad past the 1500-byte cap: a real CDN-served page that mentions
	// "cloudflare" in headers/scripts must NOT trip the heuristic.
	padding := strings.Repeat("Lorem ipsum dolor sit amet. ", 100)
	html := `<html><body><p>Welcome. We use Cloudflare to serve this page.</p>` +
		padding + `</body></html>`
	if DetectJSChallenge(html, "text/html", len(html)) {
		t.Errorf("expected DetectJSChallenge=false on >1500-byte page (len=%d)", len(html))
	}
}

func TestDetectJSChallenge_NonHTMLNegative(t *testing.T) {
	body := `{"ok": true, "challenge-platform": "ignored"}`
	if DetectJSChallenge(body, "application/json", len(body)) {
		t.Error("expected DetectJSChallenge=false when content-type is non-HTML")
	}
}
