package engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractLinks_BasicHrefAndText(t *testing.T) {
	html := `<html><body><a href="/foo">Bar</a></body></html>`
	links := ExtractLinks(html, "https://example.com/")
	if len(links) != 1 {
		t.Fatalf("len=%d, want 1: %#v", len(links), links)
	}
	if got, want := links[0].Href, "https://example.com/foo"; got != want {
		t.Errorf("Href = %q, want %q", got, want)
	}
	if got, want := links[0].Text, "Bar"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
}

func TestExtractLinks_ResolvesRelative(t *testing.T) {
	html := `<html><body>
		<a href="/abs">abs</a>
		<a href="relative">rel</a>
		<a href="../up">up</a>
	</body></html>`
	links := ExtractLinks(html, "https://example.com/dir/page")

	wantHrefs := []string{
		"https://example.com/abs",
		"https://example.com/dir/relative",
		"https://example.com/up",
	}
	if len(links) != len(wantHrefs) {
		t.Fatalf("len=%d, want %d: %#v", len(links), len(wantHrefs), links)
	}
	for i, want := range wantHrefs {
		if links[i].Href != want {
			t.Errorf("links[%d].Href = %q, want %q", i, links[i].Href, want)
		}
	}
}

func TestExtractLinks_SkipsJavascriptScheme(t *testing.T) {
	html := `<html><body><a href="javascript:void(0)">x</a><a href="/keep">k</a></body></html>`
	links := ExtractLinks(html, "https://example.com/")
	if len(links) != 1 || !strings.HasSuffix(links[0].Href, "/keep") {
		t.Fatalf("unexpected links: %#v", links)
	}
}

func TestExtractLinks_SkipsMailtoTel(t *testing.T) {
	html := `<html><body>
		<a href="mailto:a@b.com">mail</a>
		<a href="tel:+15551234567">phone</a>
		<a href="/keep">k</a>
	</body></html>`
	links := ExtractLinks(html, "https://example.com/")
	if len(links) != 1 || !strings.HasSuffix(links[0].Href, "/keep") {
		t.Fatalf("unexpected links: %#v", links)
	}
}

func TestExtractLinks_SkipsEmptyAndFragment(t *testing.T) {
	html := `<html><body>
		<a href="">empty</a>
		<a href="#section">frag</a>
		<a href="#">hash</a>
		<a href="/keep">k</a>
	</body></html>`
	links := ExtractLinks(html, "https://example.com/")
	if len(links) != 1 || !strings.HasSuffix(links[0].Href, "/keep") {
		t.Fatalf("unexpected links: %#v", links)
	}
}

func TestExtractLinks_PreservesRel(t *testing.T) {
	html := `<html><body><a href="/out" rel="nofollow noopener">o</a></body></html>`
	links := ExtractLinks(html, "https://example.com/")
	if len(links) != 1 {
		t.Fatalf("len=%d, want 1", len(links))
	}
	if got, want := links[0].Rel, "nofollow noopener"; got != want {
		t.Errorf("Rel = %q, want %q", got, want)
	}
}

func TestExtractLinks_DedupesByHrefText(t *testing.T) {
	html := `<html><body>
		<a href="/foo">Bar</a>
		<a href="/foo">Bar</a>
		<a href="/foo">Different</a>
	</body></html>`
	links := ExtractLinks(html, "https://example.com/")
	if len(links) != 2 {
		t.Fatalf("len=%d, want 2: %#v", len(links), links)
	}
	if links[0].Text != "Bar" || links[1].Text != "Different" {
		t.Errorf("texts = [%q %q], want [Bar Different]", links[0].Text, links[1].Text)
	}
}

func TestExtractLinks_TruncatesLongText(t *testing.T) {
	long := strings.Repeat("a", 300)
	html := `<html><body><a href="/foo">` + long + `</a></body></html>`
	links := ExtractLinks(html, "https://example.com/")
	if len(links) != 1 {
		t.Fatalf("len=%d, want 1", len(links))
	}
	runes := []rune(links[0].Text)
	if len(runes) != 201 {
		t.Fatalf("rune len = %d, want 201 (200 + ellipsis)", len(runes))
	}
	if runes[200] != '…' {
		t.Errorf("last rune = %q, want …", runes[200])
	}
}

func TestExtractLinks_HandlesNestedTags(t *testing.T) {
	html := `<html><body><a href="/"><span>Bold</span> <em>italic</em></a></body></html>`
	links := ExtractLinks(html, "https://example.com/")
	if len(links) != 1 {
		t.Fatalf("len=%d, want 1", len(links))
	}
	if got, want := links[0].Text, "Bold italic"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
}

// linkTestProse provides enough body text to pass readability extraction.
var linkTestProse = strings.Repeat("This page exists to exercise the link extractor end-to-end with realistic text. ", 10)

var linksTestPage = `<!doctype html><html><head><title>Links Test</title></head><body>
<article><h1>Links Test</h1>
<p>` + linkTestProse + `</p>
<nav>
  <a href="/one" rel="next">One</a>
  <a href="/two">Two</a>
  <a href="https://other.example.org/three" rel="external nofollow">Three</a>
  <a href="relative-four">Four</a>
  <a href="/five">Five</a>
</nav>
</article></body></html>`

func TestExtract_LinksFlagOn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(linksTestPage))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL+"/", Options{WithLinks: true})
	if len(result.Links) != 5 {
		t.Fatalf("len(Links) = %d, want 5: %#v", len(result.Links), result.Links)
	}

	// Spot-check first entry resolves against base server URL.
	if !strings.HasPrefix(result.Links[0].Href, srv.URL) {
		t.Errorf("Links[0].Href = %q, want prefix %q", result.Links[0].Href, srv.URL)
	}
	if result.Links[0].Text != "One" {
		t.Errorf("Links[0].Text = %q, want One", result.Links[0].Text)
	}
	if result.Links[0].Rel != "next" {
		t.Errorf("Links[0].Rel = %q, want next", result.Links[0].Rel)
	}

	// Absolute href to a different host must be preserved verbatim.
	if result.Links[2].Href != "https://other.example.org/three" {
		t.Errorf("Links[2].Href = %q, want absolute other-host", result.Links[2].Href)
	}
	if result.Links[2].Rel != "external nofollow" {
		t.Errorf("Links[2].Rel = %q, want %q", result.Links[2].Rel, "external nofollow")
	}
}

func TestExtract_LinksFlagOffDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(linksTestPage))
	}))
	defer srv.Close()

	result := FromURL(srv.URL + "/")
	if result.Links != nil {
		t.Fatalf("Links = %#v, want nil when --with-links flag is off", result.Links)
	}
}
