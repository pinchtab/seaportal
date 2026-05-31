package engine

import (
	"strings"
	"testing"
)

func TestFromHTMLWithOptions_PassesThroughFlags(t *testing.T) {
	html := `<!doctype html>
<html><head><title>Sample</title></head>
<body>
<article>
<h1>Hello</h1>
<p>This is a meaningful paragraph that should survive readability extraction without trouble. It has enough words to look like real content.</p>
<p>Another sentence to keep the article above the readability minimum threshold. Lorem ipsum dolor sit amet, consectetur adipiscing elit.</p>
<a href="/docs">Docs link</a>
<a href="https://other.example.org/x">Absolute link</a>
</article>
</body></html>`

	opts := Options{WithLinks: true}
	res := FromHTMLWithOptions(html, "https://example.com/page", opts)

	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if len(res.Links) == 0 {
		t.Fatalf("expected Links populated when WithLinks=true, got 0")
	}

	// Relative link should be resolved against the base URL.
	foundResolved := false
	for _, l := range res.Links {
		if strings.Contains(l.Href, "example.com/docs") {
			foundResolved = true
			break
		}
	}
	if !foundResolved {
		t.Errorf("expected relative link to be resolved against base-url; links: %+v", res.Links)
	}

	if !strings.Contains(strings.ToLower(res.Content), "hello") {
		t.Errorf("expected extracted content to contain heading; got %q", res.Content)
	}
}

func TestFromHTMLWithOptions_DefaultOptionsMatchFromHTML(t *testing.T) {
	html := `<!doctype html><html><head><title>T</title></head><body><article><h1>Hi</h1><p>Lorem ipsum dolor sit amet consectetur adipiscing elit. Some more text to keep readability happy.</p></article></body></html>`
	a := FromHTML(html, "https://example.com/")
	b := FromHTMLWithOptions(html, "https://example.com/", Options{})
	if a.Title != b.Title {
		t.Errorf("title mismatch: %q vs %q", a.Title, b.Title)
	}
	if a.URL != b.URL {
		t.Errorf("url mismatch")
	}
}
