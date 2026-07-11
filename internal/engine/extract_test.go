package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/seaportal/internal/engine/leakcheck"
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
	if testing.Short() {
		t.Skip("sleeps two real 1s Retry-After waits; runs in the full lane (./dev test full)")
	}
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

func TestFromURL_Markdown406RetriesAsHTML(t *testing.T) {
	leakcheck.CheckLeak(t)
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

	if !result.IsSPA && result.Length > 100 {
		t.Log("Note: Fast mode may still extract some content from minimal SPA pages")
	}
}

func TestFromURL_UnreachableHostClassified(t *testing.T) {
	result := FromURL("http://nonexistent.invalid.")
	if result.Error == "" {
		t.Fatal("expected an error on unreachable host, got none")
	}
	if result.Profile.Class == "" {
		t.Fatalf("Profile.Class is empty on error path; every Result must be classified. Error=%q", result.Error)
	}
}

func TestFromHTML_ReadabilityErrorClassified(t *testing.T) {
	result := FromHTML("", "https://example.test/")
	if result.Profile.Class == "" {
		t.Fatalf("Profile.Class is empty for empty-HTML input; Error=%q", result.Error)
	}
}

func TestExtract_LanguageFallback(t *testing.T) {
	// No <html lang>, no og:locale, no Content-Language, no JSON-LD → the
	// stopword fallback must populate Result.Language from the prose.
	html := `<!DOCTYPE html>
<html>
<head><title>Sample article</title></head>
<body>
<article>
<h1>Sample article</h1>
<p>The quick brown fox jumps over the lazy dog. This is a sentence which has
been used for many years to test fonts and keyboards. It contains every letter
of the alphabet and is one of the most famous pangrams in the English
language. There are other pangrams, but this one would always be the first
that comes to mind for those who have ever typed on a typewriter. They have
used this sentence in countless examples for decades and it has not lost its
charm. From the early days of mechanical typewriters to the modern era of
software, this pangram has been the canonical test string for new keyboards
and fonts which need to verify that every glyph is reachable.</p>
</article>
</body>
</html>`

	result := FromHTML(html, "https://example.test/article")
	if result.Error != "" {
		t.Fatalf("extraction failed: %s", result.Error)
	}
	if result.Language != "en" {
		t.Errorf("Language fallback: got %q, want %q (content length=%d)", result.Language, "en", len(result.Content))
	}
}

// ALP-036: JSON/XML bodies must pass through verbatim. The HTML→markdown path
// escapes `_`/`[`, turning "node_id" into the invalid JSON escape "node\_id".
func TestFromURL_JSONPassthroughUnescaped(t *testing.T) {
	body := `{"node_id":"R_kgDONaN_id","full_name":"pinchtab/seaportal","key_a":"v_b"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	result := FromURL(server.URL)

	if result.Error != "" {
		t.Fatalf("extraction failed: %s", result.Error)
	}
	if strings.Contains(result.Content, `\_`) {
		t.Errorf("JSON content markdown-escaped (contains \\_): %q", result.Content)
	}
	if result.Content != body {
		t.Errorf("JSON not passed through verbatim:\n got: %q\nwant: %q", result.Content, body)
	}
	// The body must still parse as JSON (no invalid \_ escapes).
	var v map[string]any
	if err := json.Unmarshal([]byte(result.Content), &v); err != nil {
		t.Errorf("extracted JSON does not round-trip: %v", err)
	}
	if result.ExtractionMethod != "raw" {
		t.Errorf("ExtractionMethod = %q, want raw", result.ExtractionMethod)
	}
}

func TestFromURL_XMLPassthroughUnescaped(t *testing.T) {
	body := `<?xml version="1.0"?><root><node_id>a_b</node_id></root>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	result := FromURL(server.URL)

	if result.Error != "" {
		t.Fatalf("extraction failed: %s", result.Error)
	}
	if strings.Contains(result.Content, `\_`) {
		t.Errorf("XML content markdown-escaped (contains \\_): %q", result.Content)
	}
	if result.Content != body {
		t.Errorf("XML not passed through verbatim:\n got: %q\nwant: %q", result.Content, body)
	}
}

// XHTML is HTML and must keep flowing through the readability/markdown path,
// not the raw JSON/XML branch.
func TestFromURL_XHTMLStillHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xhtml+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body><article><h1>Title</h1><p>Some body text here.</p></article></body></html>`))
	}))
	defer server.Close()

	result := FromURL(server.URL)
	if result.ExtractionMethod == "raw" {
		t.Errorf("XHTML routed through raw passthrough; must use the HTML path")
	}
}

// regression: ALP-038 — binary responses (image/*, octet-stream) fell through
// into readability/markdown, which parses binary bytes as HTML and stalls for
// minutes. The gate must run on the default path (no ContentTypePreflight).
func TestFromURL_BinaryContentSkippedFast(t *testing.T) {
	binaryBody := make([]byte, 128*1024)
	for i := range binaryBody {
		binaryBody[i] = byte(i * 7 % 251)
	}

	for _, ct := range []string{"image/png", "image/jpeg", "application/octet-stream"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ct)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(binaryBody)
		}))

		start := time.Now()
		result := FromURLWithOptions(server.URL, Options{})
		elapsed := time.Since(start)
		server.Close()

		want := "skipped binary content: " + ct
		if result.Error != want {
			t.Errorf("%s: expected error %q, got %q", ct, want, result.Error)
		}
		if elapsed > 5*time.Second {
			t.Errorf("%s: skip took %v; binary bytes reached the HTML pipeline", ct, elapsed)
		}
		if result.StatusCode != http.StatusOK {
			t.Errorf("%s: expected status 200 on skip result, got %d", ct, result.StatusCode)
		}
		if result.ResponseContentType != ct {
			t.Errorf("%s: expected responseContentType preserved, got %q", ct, result.ResponseContentType)
		}
	}
}

func TestSleepCtx(t *testing.T) {
	// Completes normally when the context stays alive.
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Errorf("sleepCtx(alive) = %v, want nil", err)
	}
	// Returns the context error promptly when already cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := sleepCtx(ctx, time.Hour); err == nil {
		t.Error("sleepCtx(cancelled) = nil, want context error")
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Error("sleepCtx did not return promptly on cancellation")
	}
}

// ALP-043: a retry backoff wait must be interruptible by the request context —
// an overall deadline / SIGINT should preempt an in-flight retry sleep instead
// of blocking for the full backoff. Uses a Retry-After of 1s (a deterministic
// wait, unaffected by the test's shrunk retryBackoffBase) that a 100ms deadline
// must cut short.
func TestFromURL_RetrySleepInterruptedByContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := FromURLWithOptions(server.URL, Options{Context: ctx, MaxRetries: 3})
	elapsed := time.Since(start)

	if elapsed > 800*time.Millisecond {
		t.Errorf("retry sleep not interrupted: elapsed %s (want < 800ms, the Retry-After was 1s)", elapsed)
	}
	if result.Error == "" || !strings.Contains(result.Error, "context") {
		t.Errorf("result.Error = %q, want a context cancellation error", result.Error)
	}
}

// ALP-044: a supplied/loaded schema with no fields (commonly the omitted
// top-level "fields" wrapper) must warn instead of silently producing nothing.
func TestFromURL_SchemaEmptyFieldsWarns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><h1>Title</h1><p>Body content here for extraction.</p></body></html>`))
	}))
	defer server.Close()

	empty := FromURLWithOptions(server.URL, Options{Schema: &Schema{}})
	if !containsWarning(empty.Warnings, "no fields") {
		t.Errorf("empty schema: warnings = %v, want a 'no fields' warning", empty.Warnings)
	}
	if len(empty.Schema) != 0 {
		t.Errorf("empty schema: result.Schema = %v, want empty", empty.Schema)
	}

	// A valid schema still populates result.Schema with no spurious warning.
	valid := FromURLWithOptions(server.URL, Options{Schema: &Schema{Fields: map[string]FieldSpec{"title": {Selector: "h1"}}}})
	if containsWarning(valid.Warnings, "no fields") {
		t.Errorf("valid schema: unexpected 'no fields' warning: %v", valid.Warnings)
	}
	if valid.Schema["title"] != "Title" {
		t.Errorf("valid schema: result.Schema[title] = %v, want Title", valid.Schema["title"])
	}
}

// regression: ALP-046 — the shared waitAndRetry tail must enforce
// totalRetryTimeout: a Retry-After larger than the remaining budget stops the
// retry loop immediately instead of sleeping.
func TestRetryTotalBudgetEnforced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	var outcomes []string
	opts := Options{
		MaxRetries:        3,
		MaxRetryWait:      5 * time.Second,
		TotalRetryTimeout: 500 * time.Millisecond,
		RetryLogger:       func(e RetryEvent) { outcomes = append(outcomes, e.Outcome) },
	}
	start := time.Now()
	result := FromURLWithOptions(server.URL, opts)
	elapsed := time.Since(start)

	if elapsed > 1500*time.Millisecond {
		t.Errorf("budget-exceeded retry took %v; must return without sleeping", elapsed)
	}
	if result.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want 503 handed back on budget exhaustion", result.StatusCode)
	}
	if result.RetryCount != 0 {
		t.Errorf("RetryCount = %d, want 0 (no retry inside exhausted budget)", result.RetryCount)
	}
	if len(outcomes) == 0 || outcomes[len(outcomes)-1] != "timeout" {
		t.Errorf("retry outcomes = %v, want final \"timeout\" event", outcomes)
	}
}

// ALP-047: a per-domain DomainRetry{MaxRetryWait} override must reach the retry
// stage and clamp the backoff. Without the fix the stage defaulted to 60s and
// ignored the override, so backoff grew exponentially past the cap.
func TestFromURL_DomainMaxRetryWaitClampsBackoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // 503, no Retry-After → exponential backoff
	}))
	defer server.Close()

	const capWait = 5 * time.Millisecond
	var waits []time.Duration
	opts := Options{
		DomainRetryConfig: map[string]DomainRetry{
			"127.0.0.1": {MaxRetries: 3, MaxRetryWait: capWait},
		},
		RetryLogger: func(e RetryEvent) {
			if e.Outcome == "retrying" {
				waits = append(waits, e.WaitTime)
			}
		},
	}
	_ = FromURLWithOptions(server.URL, opts)

	if len(waits) < 2 {
		t.Fatalf("expected multiple retry waits, got %d: %v", len(waits), waits)
	}
	// Clamped backoff is at most capWait × addJitter's 1.25 ceiling; the
	// exponential base would otherwise reach ~20ms by the third attempt.
	limit := capWait*5/4 + time.Millisecond
	for i, w := range waits {
		if w > limit {
			t.Errorf("wait[%d] = %s exceeds domain cap %s (limit %s) — override not applied to the stage", i, w, capWait, limit)
		}
	}
}
