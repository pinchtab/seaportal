package engine

import (
	"testing"
)

func TestExtract_CanonicalURLPopulated(t *testing.T) {
	html := `<!doctype html><html><head>
<link rel="canonical" href="https://example.com/canonical-path">
<title>Canonical Test</title>
</head><body>
<article><h1>Canonical Test</h1>
<p>` + longProse() + `</p>
</article></body></html>`

	srv := newSiteServer(t, map[string]string{"/messy": html})

	result := FromURL(srv.URL + "/messy?utm_source=twitter&utm_medium=social&id=42")
	if result.CanonicalURL != "https://example.com/canonical-path" {
		t.Fatalf("CanonicalURL = %q, want %q", result.CanonicalURL, "https://example.com/canonical-path")
	}
}

func TestExtract_CanonicalURL_AlgorithmicFallback(t *testing.T) {
	html := `<!doctype html><html><head><title>No Canonical</title></head><body>
<article><h1>No Canonical</h1>
<p>` + longProse() + `</p>
</article></body></html>`

	srv := newSiteServer(t, map[string]string{"/post": html})

	result := FromURL(srv.URL + "/post?utm_source=tw&id=42")
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
