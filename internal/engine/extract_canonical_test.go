package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestExtract_CanonicalURLPopulated verifies the wiring: a fetch URL with
// tracking params + an HTML <link rel="canonical"> surfaces the link's value
// on Result.CanonicalURL.
func TestExtract_CanonicalURLPopulated(t *testing.T) {
	html := `<!doctype html><html><head>
<link rel="canonical" href="https://example.com/canonical-path">
<title>Canonical Test</title>
</head><body>
<article><h1>Canonical Test</h1>
<p>` + longProse() + `</p>
</article></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	result := FromURL(srv.URL + "/messy?utm_source=twitter&utm_medium=social&id=42")
	if result.CanonicalURL != "https://example.com/canonical-path" {
		t.Fatalf("CanonicalURL = %q, want %q", result.CanonicalURL, "https://example.com/canonical-path")
	}
}

// TestExtract_CanonicalURL_AlgorithmicFallback verifies that when no
// <link rel="canonical"> is present, the algorithmic canonicalisation kicks
// in and strips tracking params from the fetch URL.
func TestExtract_CanonicalURL_AlgorithmicFallback(t *testing.T) {
	html := `<!doctype html><html><head><title>No Canonical</title></head><body>
<article><h1>No Canonical</h1>
<p>` + longProse() + `</p>
</article></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	result := FromURL(srv.URL + "/post?utm_source=tw&id=42")
	// Canonical should drop utm_source but keep id=42; differs from raw URL.
	if result.CanonicalURL == "" {
		t.Fatalf("expected CanonicalURL to be set, got empty")
	}
	if result.CanonicalURL == result.URL {
		t.Fatalf("CanonicalURL should differ from raw URL")
	}
}

func longProse() string {
	s := ""
	for i := 0; i < 20; i++ {
		s += "This is a paragraph of sample prose used for extraction testing. "
	}
	return s
}
