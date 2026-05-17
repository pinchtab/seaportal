package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractMetadata_OGFields(t *testing.T) {
	html := `<html><head>
<meta property="og:title" content="T">
<meta property="og:description" content="Desc">
<meta property="og:image" content="https://x/y.jpg">
<meta property="og:locale" content="en_US">
<meta property="og:type" content="article">
</head></html>`
	m := ExtractMetadata(html)
	if m.Description != "Desc" {
		t.Errorf("Description: got %q", m.Description)
	}
	if m.ImageURL != "https://x/y.jpg" {
		t.Errorf("ImageURL: got %q", m.ImageURL)
	}
	if m.Language != "en_US" {
		t.Errorf("Language: got %q", m.Language)
	}
	if m.OGType != "article" {
		t.Errorf("OGType: got %q", m.OGType)
	}
}

func TestExtractMetadata_ArticleFields(t *testing.T) {
	html := `<head>
<meta property="article:author" content="Jane Doe">
<meta property="article:published_time" content="2024-01-02T03:04:05Z">
<meta property="article:section" content="World">
</head>`
	m := ExtractMetadata(html)
	if m.Author != "Jane Doe" {
		t.Errorf("Author: got %q", m.Author)
	}
	if m.PublishedDate != "2024-01-02T03:04:05Z" {
		t.Errorf("PublishedDate: got %q", m.PublishedDate)
	}
	if m.Section != "World" {
		t.Errorf("Section: got %q", m.Section)
	}
}

func TestExtractMetadata_DublinCore(t *testing.T) {
	html := `<head>
<meta name="DC.creator" content="DC Author">
<meta name="DC.date" content="2020-05-05">
<meta name="DC.language" content="en">
<meta name="DC.subject" content="Science">
<meta name="DC.description" content="DC desc">
</head>`
	m := ExtractMetadata(html)
	if m.Author != "DC Author" {
		t.Errorf("Author: got %q", m.Author)
	}
	if m.PublishedDate != "2020-05-05" {
		t.Errorf("PublishedDate: got %q", m.PublishedDate)
	}
	if m.Language != "en" {
		t.Errorf("Language: got %q", m.Language)
	}
	if m.Section != "Science" {
		t.Errorf("Section: got %q", m.Section)
	}
	if m.Description != "DC desc" {
		t.Errorf("Description: got %q", m.Description)
	}
	if m.Keywords != "Science" {
		t.Errorf("Keywords (DC.subject fallback): got %q", m.Keywords)
	}
}

func TestExtractMetadata_AuthorPriority(t *testing.T) {
	html := `<head>
<meta property="article:author" content="Article Author">
<meta name="author" content="Name Author">
<meta name="DC.creator" content="DC Author">
<meta name="citation_author" content="Citation Author">
</head>`
	if got := ExtractMetadata(html).Author; got != "Article Author" {
		t.Errorf("article wins: got %q", got)
	}

	html2 := `<head>
<meta name="author" content="Name Author">
<meta name="DC.creator" content="DC Author">
<meta name="citation_author" content="Citation Author">
</head>`
	if got := ExtractMetadata(html2).Author; got != "Name Author" {
		t.Errorf("name=author wins over DC: got %q", got)
	}

	html3 := `<head>
<meta name="DC.creator" content="DC Author">
<meta name="citation_author" content="Citation Author">
</head>`
	if got := ExtractMetadata(html3).Author; got != "DC Author" {
		t.Errorf("DC wins over citation: got %q", got)
	}

	html4 := `<head>
<meta name="citation_author" content="Cite One">
<meta name="citation_author" content="Cite Two">
</head>`
	if got := ExtractMetadata(html4).Author; got != "Cite One; Cite Two" {
		t.Errorf("citation join: got %q", got)
	}
}

func TestExtractMetadata_ImageURL(t *testing.T) {
	og := `<meta property="og:image" content="https://og/img.png"><meta name="twitter:image" content="https://tw/img.png">`
	if got := ExtractMetadata(og).ImageURL; got != "https://og/img.png" {
		t.Errorf("og wins: got %q", got)
	}
	tw := `<meta name="twitter:image" content="https://tw/img.png">`
	if got := ExtractMetadata(tw).ImageURL; got != "https://tw/img.png" {
		t.Errorf("twitter fallback: got %q", got)
	}
}

func TestExtractMetadata_HandlesMultipleAuthors(t *testing.T) {
	html := `<head>
<meta property="article:author" content="Alice">
<meta property="article:author" content="Bob">
<meta property="article:author" content="Carol">
</head>`
	if got := ExtractMetadata(html).Author; got != "Alice; Bob; Carol" {
		t.Errorf("join: got %q", got)
	}
}

func TestExtractMetadata_UnescapesEntities(t *testing.T) {
	html := `<meta property="og:description" content="Tom &amp; Jerry&#39;s show">`
	if got := ExtractMetadata(html).Description; got != "Tom & Jerry's show" {
		t.Errorf("unescape: got %q", got)
	}
}

func TestApplyMetadata_FillsEmptyOnly(t *testing.T) {
	r := &Result{Byline: "Pre Author", Description: "Pre Desc", ImageURL: "https://pre/i.jpg"}
	applyMetadata(r, Metadata{Author: "Meta", Description: "Meta D", ImageURL: "https://meta/i.jpg"})
	if r.Byline != "Pre Author" {
		t.Errorf("Byline overwritten: %q", r.Byline)
	}
	if r.Description != "Pre Desc" {
		t.Errorf("Description overwritten: %q", r.Description)
	}
	if r.ImageURL != "https://pre/i.jpg" {
		t.Errorf("ImageURL overwritten: %q", r.ImageURL)
	}
}

func TestApplyMetadata_RespectsJSONLDPriority(t *testing.T) {
	r := &Result{Byline: "JSONLD Author", Language: "en", Section: "JSONLD Sec"}
	applyMetadata(r, Metadata{Author: "Meta Author", Language: "de", Section: "Meta Sec", Description: "MD"})
	if r.Byline != "JSONLD Author" {
		t.Errorf("Byline overwritten: %q", r.Byline)
	}
	if r.Language != "en" {
		t.Errorf("Language overwritten: %q", r.Language)
	}
	if r.Section != "JSONLD Sec" {
		t.Errorf("Section overwritten: %q", r.Section)
	}
	if r.Description != "MD" {
		t.Errorf("Description should fill empty: %q", r.Description)
	}
}

func TestApplyMetadata_AuthorsPrependIdempotent(t *testing.T) {
	r := &Result{Content: "body"}
	applyMetadata(r, Metadata{Author: "X"})
	first := r.Content
	if !strings.HasPrefix(first, "**Authors:** X\n\n") {
		t.Fatalf("expected prepend, got %q", first)
	}
	// second pass on same content should NOT double-prepend
	applyMetadata(r, Metadata{Author: "X"})
	if r.Content != first {
		t.Errorf("double-prepended: %q", r.Content)
	}
}

func TestExtract_OGFullFixture(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "static", "article-og-full.html")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	result := FromHTML(string(raw), "https://example.com/og-demo")

	if !strings.Contains(result.Byline, "Alice Smith") || !strings.Contains(result.Byline, "Bob Jones") {
		t.Errorf("Byline missing article:author values: %q", result.Byline)
	}
	if result.Description != "A description from OG." {
		t.Errorf("Description: got %q", result.Description)
	}
	if result.ImageURL != "https://example.com/cover.jpg" {
		t.Errorf("ImageURL: got %q", result.ImageURL)
	}
	if result.Language != "en_US" {
		t.Errorf("Language: got %q", result.Language)
	}
	if result.Section != "Tech" {
		t.Errorf("Section: got %q", result.Section)
	}
	if result.PublishedDate != "2024-04-01T12:00:00Z" {
		t.Errorf("PublishedDate: got %q", result.PublishedDate)
	}
}
