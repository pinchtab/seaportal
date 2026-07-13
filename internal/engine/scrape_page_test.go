package engine

import (
	"net/url"
	"strings"
	"testing"
)

const articleHTML = `<html><head>
<title>My Great Article</title>
<meta name="description" content="An article about interesting things">
<meta property="og:type" content="article">
<meta property="og:image" content="https://ex.com/img.png">
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"Article","headline":"My Great Article","author":"Jane Doe","inLanguage":"en"}
</script>
</head><body>
<h1>My Great Article</h1>
<p>Body text goes here with enough words to extract properly and produce readable markdown content for the result object.</p>
<a href="/related">Related</a>
<a href="/about">About</a>
<a href="https://external.example/x">External</a>
</body></html>`

const productHTML = `<html><head>
<title>Cool Widget</title>
<meta property="og:type" content="product">
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"Product","url":"https://ex.com/widget"}
</script>
</head><body>
<h1>Cool Widget</h1>
<p>A great product with a detailed description of its many fine features.</p>
<a href="/cart">Add to cart</a>
</body></html>`

func mustBase(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse base %q: %v", s, err)
	}
	return u
}

func TestAssemblePageArticle(t *testing.T) {
	base := mustBase(t, "https://ex.com")
	target := "https://ex.com/blog/my-article"
	r := FromHTMLWithOptions(articleHTML, target, Options{WithLinks: true})

	p := assemblePage(base, target, r, false)

	if p.URL != target {
		t.Errorf("URL = %s, want %s", p.URL, target)
	}
	if p.Markdown == "" {
		t.Error("markdown is empty")
	}
	if p.Meta["title"] == "" || p.Meta["description"] == "" {
		t.Errorf("meta under-populated: %v", p.Meta)
	}
	if p.Meta["author"] != "Jane Doe" {
		t.Errorf("meta author = %q, want Jane Doe (JSON-LD via Result)", p.Meta["author"])
	}
	if len(p.Schema) == 0 {
		t.Error("schema (JSON-LD) not captured")
	}
	if p.ContentType != "article" {
		t.Errorf("contentType = %q, want article", p.ContentType)
	}
	if p.InternalLinks != 2 {
		t.Errorf("internalLinks = %d, want 2", p.InternalLinks)
	}
	if p.ExternalLinks != 1 {
		t.Errorf("externalLinks = %d, want 1", p.ExternalLinks)
	}
	if p.Performance != nil {
		t.Errorf("performance should be nil without WithPerformance, got %+v", p.Performance)
	}
}

func TestAssemblePageProduct(t *testing.T) {
	base := mustBase(t, "https://ex.com")
	target := "https://ex.com/widget"
	r := FromHTMLWithOptions(productHTML, target, Options{WithLinks: true})
	r.ContentLength = int64(len(productHTML))
	r.TTFBMs = 12

	p := assemblePage(base, target, r, true)

	if p.ContentType != "product" {
		t.Errorf("contentType = %q, want product", p.ContentType)
	}
	if len(p.Schema) == 0 || strings.ToLower(toStr(p.Schema[0]["type"])) != "product" {
		t.Errorf("schema not captured as product: %v", p.Schema)
	}
	if p.InternalLinks != 1 {
		t.Errorf("internalLinks = %d, want 1", p.InternalLinks)
	}
	if p.Performance == nil {
		t.Fatal("performance should be populated with WithPerformance")
	}
	if p.Performance.Requests != 1 {
		t.Errorf("performance.Requests = %d, want 1", p.Performance.Requests)
	}
	if p.Performance.TotalBytes != int64(len(productHTML)) {
		t.Errorf("performance.TotalBytes = %d, want %d (Result.ContentLength)", p.Performance.TotalBytes, len(productHTML))
	}
	if p.Performance.TTFBMillis != 12 {
		t.Errorf("performance.TTFBMillis = %d, want 12 (Result.TTFBMs)", p.Performance.TTFBMillis)
	}
}

func TestAssemblePageFailedStillPopulatesStatusAndError(t *testing.T) {
	base := mustBase(t, "https://ex.com")
	target := "https://ex.com/gone"
	r := Result{StatusCode: 500, Error: "server error"}

	p := assemblePage(base, target, r, false)

	if p.Status != 500 || p.Error != "server error" {
		t.Errorf("failed page = {status:%d error:%q}, want {500, server error}", p.Status, p.Error)
	}
	if p.ContentType != "unknown" {
		t.Errorf("contentType = %q, want unknown for an empty extraction", p.ContentType)
	}
}

func TestClassifyContentTypeFromResult(t *testing.T) {
	if got := classifyContentType(Result{ResponseHeaders: ResponseHeaders{LDJSONBlocks: []LDJSONBlock{{Type: "NewsArticle"}}}, Content: "x"}, ""); got != "article" {
		t.Errorf("NewsArticle = %q, want article", got)
	}
	if got := classifyContentType(Result{ResponseHeaders: ResponseHeaders{LDJSONBlocks: []LDJSONBlock{{Type: "Product"}}}, Content: "x"}, "https://ex.com/w"); got != "product" {
		t.Errorf("Product = %q, want product", got)
	}
	if got := classifyContentType(Result{}, "https://ex.com/docs/x"); got != "unknown" {
		t.Errorf("empty content = %q, want unknown", got)
	}
}

func TestStructuralContentTypeFallback(t *testing.T) {
	cases := []struct {
		name string
		url  string
		r    Result
		want string
	}{
		{"mdn-docs-url", "https://developer.mozilla.org/en-US/docs/Web/JavaScript", Result{Content: "text"}, "article"},
		{"blog-url", "https://ex.com/blog/hello", Result{Content: "text"}, "article"},
		{"prose-density", "https://ex.com/x", Result{Content: "text", HeadingCount: 1, ParagraphCount: 5}, "article"},
		{"plain-page", "https://ex.com/", Result{Content: "hi", ParagraphCount: 1}, "page"},
		{"empty-body", "https://ex.com/docs/x", Result{}, "unknown"},
	}
	for _, c := range cases {
		if got := classifyContentType(c.r, c.url); got != c.want {
			t.Errorf("%s: classify = %q, want %q", c.name, got, c.want)
		}
	}
}

func toStr(v any) string {
	s, _ := v.(string)
	return s
}
