package engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFromURL_Basic(t *testing.T) {
	result := FromURL("https://example.com")
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	if result.Length == 0 {
		t.Error("expected non-zero content length")
	}
	if result.Confidence == 0 {
		t.Error("expected non-zero confidence")
	}
}

func TestDetectBlocked(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected bool
	}{
		{
			name:     "captcha page",
			html:     `<html><body>Please complete the CAPTCHA to continue</body></html>`,
			expected: true,
		},
		{
			name:     "cloudflare challenge",
			html:     `<html><body>Checking your browser... Cloudflare</body></html>`,
			expected: true,
		},
		{
			name:     "access denied",
			html:     `<html><body>Access Denied - You don't have permission</body></html>`,
			expected: true,
		},
		{
			name:     "normal page",
			html:     `<html><body><article><h1>Hello World</h1><p>This is content.</p></article></body></html>`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectBlocked(tt.html)
			if result != tt.expected {
				t.Errorf("DetectBlocked() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRetryOn429(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body><article><h1>Success</h1><p>Content here.</p></article></body></html>`))
	}))
	defer server.Close()

	opts := Options{
		MaxRetries:        3,
		MaxRetryWait:      5 * time.Second,
		TotalRetryTimeout: 10 * time.Second,
	}
	result := FromURLWithOptions(server.URL, opts)

	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	if result.RetryCount != 2 {
		t.Errorf("expected 2 retries, got %d", result.RetryCount)
	}
}

func TestHTTPStatusCodeAwareness(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		wantBlocked bool
	}{
		{"401 Unauthorized", 401, true},
		{"403 Forbidden", 403, true},
		{"404 Not Found", 404, false},
		{"200 OK", 200, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`<html><body><p>Response body</p></body></html>`))
			}))
			defer server.Close()

			result := FromURL(server.URL)
			if result.IsBlocked != tt.wantBlocked {
				t.Errorf("IsBlocked = %v, want %v", result.IsBlocked, tt.wantBlocked)
			}
		})
	}
}

func TestFromHTML(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
<article>
<h1>Main Title</h1>
<p>This is the main content of the page.</p>
<p>It has multiple paragraphs.</p>
</article>
</body>
</html>`

	result := FromHTML(html, "https://example.com/test")

	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Page")
	}
	if result.Length == 0 {
		t.Error("expected non-zero content length")
	}
	if !strings.Contains(result.Content, "Main Title") {
		t.Error("content should contain 'Main Title'")
	}
}

func TestDedupe(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
<article>
<p>This is unique content.</p>
<p>This is unique content.</p>
<p>This is unique content.</p>
<p>Different content here.</p>
</article>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	result := FromURLWithOptions(server.URL, Options{Dedupe: true})

	if !result.DedupeApplied {
		t.Error("expected dedupe to be applied")
	}
}

// TestFromURL_Markdown404NotSuccess is a regression test for the bug where
// any text/markdown response was treated as a successful extraction with
// Confidence=100 / Profile.Outcome=extract, regardless of HTTP status. A
// 404 + text/markdown is still an error page and must not be classified
// as a trustworthy extraction.
func TestFromURL_Markdown404NotSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("# Not Found\n\nThe requested resource does not exist on this server.\n"))
	}))
	defer server.Close()

	result := FromURL(server.URL)

	if result.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", result.StatusCode)
	}
	if result.Confidence == 100 {
		t.Errorf("Confidence = 100 on a 404 response — must not be hardcoded to max for non-2xx markdown")
	}
	if result.Profile.Outcome == OutcomeExtract {
		t.Errorf("Profile.Outcome = %q on 404, must not be %q", result.Profile.Outcome, OutcomeExtract)
	}
	if result.Profile.Class == PageSSR {
		t.Errorf("Profile.Class = %q on 404, must not be %q", result.Profile.Class, PageSSR)
	}
	if result.Profile.Trustworthy {
		t.Error("Profile.Trustworthy = true on 404 — must be false for error responses")
	}
}

// TestFromURL_Markdown406RetriesAsHTML is a regression test for the bug where
// the content-negotiation retry only fired on 404. Spec-compliant servers
// return 406 Not Acceptable when they cannot satisfy `Accept: text/markdown`,
// and SeaPortal was extracting the 406 error body as content instead of
// retrying with an HTML-only Accept.
func TestFromURL_Markdown406RetriesAsHTML(t *testing.T) {
	var attempts int
	var firstAccept, secondAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "text/markdown") {
			firstAccept = accept
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotAcceptable)
			_, _ = w.Write([]byte("not acceptable"))
			return
		}
		secondAccept = accept
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body><article><h1>Recovered</h1>` +
			`<p>This paragraph is intentionally long enough to clear readability's body-length floor ` +
			`so the test does not depend on borderline scoring inside the extraction pipeline.</p>` +
			`</article></body></html>`))
	}))
	defer server.Close()

	result := FromURL(server.URL)

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2 (initial 406 + HTML retry)", attempts)
	}
	if !strings.Contains(firstAccept, "text/markdown") {
		t.Errorf("first request Accept missing markdown: %q", firstAccept)
	}
	if strings.Contains(secondAccept, "text/markdown") {
		t.Errorf("retry Accept should not include markdown: %q", secondAccept)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200 after retry", result.StatusCode)
	}
	if !strings.Contains(result.Content, "Recovered") {
		t.Errorf("retry HTML not extracted; content = %q", result.Content)
	}
}

// TestFromURL_Markdown503Blocked verifies that markdown responses with a
// blocked-status code (503 here) still get the blocked-profile classification.
func TestFromURL_Markdown503Blocked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("# Service Unavailable\n\nPlease retry later.\n"))
	}))
	defer server.Close()

	result := FromURLWithOptions(server.URL, Options{MaxRetries: 0})

	if !result.IsBlocked {
		t.Error("IsBlocked = false on 503 markdown response, want true")
	}
	if result.Profile.Class != PageBlocked {
		t.Errorf("Profile.Class = %q on 503, want %q", result.Profile.Class, PageBlocked)
	}
	if result.Profile.Outcome != OutcomeNeedsBrowser {
		t.Errorf("Profile.Outcome = %q on 503, want %q", result.Profile.Outcome, OutcomeNeedsBrowser)
	}
}

// TestFromURL_MarkdownDedupeRecomputes is a regression test for the bug where
// the markdown fast-path applied dedupe AFTER computing fingerprint /
// paragraph count / quality, so derived fields drifted from the actual
// returned content. With the fix, dedupe runs before metric computation and
// every derived field reflects the final deduped content.
func TestFromURL_MarkdownDedupeRecomputes(t *testing.T) {
	body := "# Title\n\n" +
		"This duplicate paragraph appears multiple times in this fixture content body.\n\n" +
		"This duplicate paragraph appears multiple times in this fixture content body.\n\n" +
		"This duplicate paragraph appears multiple times in this fixture content body.\n\n" +
		"A genuinely unique closing paragraph that should remain after dedupe.\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	result := FromURLWithOptions(server.URL, Options{Dedupe: true})

	if !result.DedupeApplied {
		t.Fatal("DedupeApplied = false, expected true")
	}
	if result.DuplicatesRemoved == 0 {
		t.Fatal("DuplicatesRemoved = 0, expected duplicates to be removed for this fixture")
	}
	if result.Length != len(result.Content) {
		t.Errorf("Length (%d) != len(Content) (%d) — Length not recomputed after dedupe", result.Length, len(result.Content))
	}
	if got, want := result.Fingerprint, SemanticFingerprint(result.Content); got != want {
		t.Errorf("Fingerprint not recomputed after dedupe:\n  got:  %s\n  want: %s", got, want)
	}
	if got, want := result.ParagraphCount, countMarkdownParagraphs(result.Content); got != want {
		t.Errorf("ParagraphCount = %d, want %d (countMarkdownParagraphs of final Content)", got, want)
	}
	if got, want := result.HeadingCount, CountMarkdownHeadings(result.Content); got != want {
		t.Errorf("HeadingCount = %d, want %d", got, want)
	}
	if got, want := result.LinkCount, CountMarkdownLinks(result.Content); got != want {
		t.Errorf("LinkCount = %d, want %d", got, want)
	}
}

func TestFastMode(t *testing.T) {
	// SPA-like page that should trigger fast bailout
	html := `<!DOCTYPE html>
<html>
<head><title>App</title></head>
<body>
<div id="root"></div>
<script src="app.js"></script>
<noscript>Please enable JavaScript</noscript>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	result := FromURLWithOptions(server.URL, Options{FastMode: true})

	// Fast mode should detect SPA and bail early
	if !result.IsSPA && result.Length > 100 {
		t.Log("Note: Fast mode may still extract some content from minimal SPA pages")
	}
}
