package engine

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// headOnlyHTML returns a small HTML head with the requested overrides.
func headOnlyHTML(extra string) string {
	return `<!doctype html><html lang="en"><head>
<meta charset="utf-8">
<title>Head Only Title</title>
` + extra + `
</head><body><p>This body must not show up in Result.Content.</p></body></html>`
}

func TestHeadOnly_ParsesTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(headOnlyHTML("")))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL, Options{HeadOnly: true})
	if result.Title != "Head Only Title" {
		t.Fatalf("title=%q want %q (err=%q)", result.Title, "Head Only Title", result.Error)
	}
	if !result.HeadOnly {
		t.Fatalf("HeadOnly flag not set")
	}
}

func TestHeadOnly_ParsesOGFields(t *testing.T) {
	og := `
<meta property="og:description" content="An og description">
<meta property="og:image" content="https://example.com/og.png">
<meta property="article:section" content="Tech">
<meta property="og:locale" content="en_US">
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(headOnlyHTML(og)))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL, Options{HeadOnly: true})
	if result.Description != "An og description" {
		t.Errorf("description=%q", result.Description)
	}
	if result.ImageURL != "https://example.com/og.png" {
		t.Errorf("imageUrl=%q", result.ImageURL)
	}
	if result.Section != "Tech" {
		t.Errorf("section=%q", result.Section)
	}
	if result.Language != "en_US" {
		t.Errorf("language=%q", result.Language)
	}
}

func TestHeadOnly_ParsesCanonical(t *testing.T) {
	extra := `<link rel="canonical" href="https://example.com/x">`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(headOnlyHTML(extra)))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL, Options{HeadOnly: true})
	if result.CanonicalURL != "https://example.com/x" {
		t.Fatalf("canonicalURL=%q", result.CanonicalURL)
	}
}

func TestHeadOnly_RespectsRangeWhenSent(t *testing.T) {
	var sawRange atomic.Bool
	var sawIdentity atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			sawRange.Store(true)
		}
		if r.Header.Get("Accept-Encoding") == "identity" {
			sawIdentity.Store(true)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Range", "bytes 0-16383/50000")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(headOnlyHTML("")))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL, Options{HeadOnly: true})
	if !sawRange.Load() {
		t.Errorf("server did not see a Range header")
	}
	if !sawIdentity.Load() {
		t.Errorf("server did not see Accept-Encoding: identity")
	}
	if result.StatusCode != http.StatusPartialContent {
		t.Errorf("statusCode=%d want 206", result.StatusCode)
	}
	if result.Title != "Head Only Title" {
		t.Errorf("title=%q", result.Title)
	}
}

func TestHeadOnly_CapsAt16KBWhenServerIgnoresRange(t *testing.T) {
	// Server ignores Range and returns ~50 KB with a sentinel string placed
	// well past the 16 KB cap. Reading it would yield a parseable canonical;
	// since head-only caps the read, the sentinel must NOT be reflected.
	bigPadding := strings.Repeat("X", 30000)
	sentinelCanonical := `<link rel="canonical" href="https://example.com/SHOULD-NOT-APPEAR">`
	page := `<!doctype html><html><head>
<title>Capped</title>
` + sentinelCanonical + `
</head><body><div>` + bigPadding + `</div></body></html>`
	// Place a *second* canonical-like marker beyond 16 KB so we can prove the
	// reader truly stopped early. To do that, push canonical to the back.
	prefix := `<!doctype html><html><head>
<title>Capped</title>
` + strings.Repeat(" ", headOnlyByteCap) + `
` + sentinelCanonical + `
</head><body></body></html>`

	var sentBytes int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ignore Range entirely.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		n, _ := w.Write([]byte(prefix))
		atomic.AddInt64(&sentBytes, int64(n))
	}))
	defer srv.Close()

	_ = page // keep the more readable variant referenced
	result := FromURLWithOptions(srv.URL, Options{HeadOnly: true})

	if result.CanonicalURL == "https://example.com/SHOULD-NOT-APPEAR" {
		t.Fatalf("canonical sentinel beyond 16 KB was parsed — read cap not enforced")
	}
	if result.Title != "Capped" {
		t.Errorf("title=%q (err=%q)", result.Title, result.Error)
	}
	if atomic.LoadInt64(&sentBytes) <= headOnlyByteCap {
		t.Errorf("test setup: server sent only %d bytes — expected > 16 KB", sentBytes)
	}
}

func TestHeadOnly_SetsHeadOnlyFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(headOnlyHTML("")))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL, Options{HeadOnly: true})
	if !result.HeadOnly {
		t.Fatal("Result.HeadOnly == false")
	}
}

func TestHeadOnly_EmptyContent(t *testing.T) {
	// Include a meta author to force applyMetadata's content-prepend path —
	// head-only must zero Content/Length AFTER metadata so the prepend is
	// discarded.
	extra := `<meta name="author" content="Jane Doe">`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(headOnlyHTML(extra)))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL, Options{HeadOnly: true})
	if result.Content != "" {
		t.Errorf("Content not empty: %q", result.Content)
	}
	if result.Length != 0 {
		t.Errorf("Length=%d want 0", result.Length)
	}
	if result.Byline != "Jane Doe" {
		t.Errorf("Byline=%q want Jane Doe", result.Byline)
	}
}

func TestExtract_HeadOnlyFlag(t *testing.T) {
	// Integration: HeadOnly=true must short-circuit the full FromURLWithOptions
	// pipeline. We assert by providing a fully-stocked head, then confirming
	// that body-only fields (HeadingCount, ParagraphCount) stay zero.
	extra := `
<meta property="og:description" content="integration-desc">
<link rel="canonical" href="https://example.com/canonical">
`
	body := strings.Repeat(`<h1>Heading</h1><p>Paragraph</p>`, 50)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html><html><head><title>I</title>%s</head><body>%s</body></html>`, extra, body)
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL, Options{HeadOnly: true})
	if !result.HeadOnly {
		t.Fatal("HeadOnly not set")
	}
	if result.Content != "" || result.Length != 0 {
		t.Fatalf("body extraction ran: content len=%d", len(result.Content))
	}
	if result.HeadingCount != 0 || result.ParagraphCount != 0 {
		t.Errorf("heading/paragraph counts populated (%d/%d) — extraction pipeline ran",
			result.HeadingCount, result.ParagraphCount)
	}
	if result.Description != "integration-desc" {
		t.Errorf("description=%q", result.Description)
	}
	if result.CanonicalURL != "https://example.com/canonical" {
		t.Errorf("canonical=%q", result.CanonicalURL)
	}
}
