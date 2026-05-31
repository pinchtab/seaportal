package engine

import (
	"encoding/xml"
	"os"
	"testing"
)

func FuzzExtractFromHTML(f *testing.F) {
	f.Add("<html><body><p>Hello world</p></body></html>")
	f.Add("<html><head><title>Test</title></head><body><article><p>Content here</p></article></body></html>")
	f.Add("")
	f.Add("<div>unclosed")
	f.Add("<script>alert('xss')</script><p>safe</p>")
	f.Add("not html at all — just plain text")

	f.Fuzz(func(t *testing.T, html string) {
		_, _ = ExtractFromHTML(html, "https://example.com")
	})
}

func FuzzDedupe(f *testing.F) {
	f.Add("Hello world\n\nHello world\n\nSomething else")
	f.Add("")
	f.Add("# Heading\n\nParagraph\n\n# Heading\n\nParagraph")
	f.Add("single line")

	f.Fuzz(func(t *testing.T, content string) {
		result := Dedupe(content)
		if result.DuplicatesFound < 0 {
			t.Fatalf("negative duplicate count: %d", result.DuplicatesFound)
		}
	})
}

func FuzzCleanupMarkdown(f *testing.F) {
	f.Add("# Title\n\nSome **bold** text\n\n---\n\n[link](https://example.com)")
	f.Add("")
	f.Add("   \n\n\n   \n\n")
	f.Add("```code block```")

	f.Fuzz(func(t *testing.T, md string) {
		_ = CleanupMarkdown(md)
	})
}

func FuzzBuildSnapshot(f *testing.F) {
	f.Add("<html><body><button>Click me</button><input type='text' placeholder='Name'></body></html>")
	f.Add("<html><body><nav><a href='/'>Home</a></nav></body></html>")
	f.Add("")
	f.Add("<div><div><div><div>deeply nested</div></div></div></div>")

	f.Fuzz(func(t *testing.T, html string) {
		_, _ = BuildSnapshot(html)
	})
}

func FuzzSanitize(f *testing.F) {
	f.Add("<html><body><p>Hello</p></body></html>")
	f.Add("<script>evil()</script>")
	f.Add("")
	f.Add("<a href='javascript:alert(1)'>x</a>")
	f.Add("<div hidden><span aria-hidden='true'>x</span></div>")
	for _, path := range []string{
		"../../testdata/static/article-ldjson.html",
		"../../testdata/static/github-awesome.html",
	} {
		if b, err := os.ReadFile(path); err == nil {
			f.Add(string(b))
		}
	}

	f.Fuzz(func(t *testing.T, html string) {
		_ = SanitizeHTML(html)
	})
}

func FuzzClassify(f *testing.F) {
	f.Add("")
	f.Add("Hello world")
	f.Add("<html><body><p>Short SSR page</p></body></html>")
	f.Add("<div id='root'></div><script>window.__NEXT_DATA__={}</script>")
	f.Add("<noscript>Please enable JavaScript</noscript>")
	f.Add("<article><h1>Title</h1><p>Some content body text.</p></article>")

	f.Fuzz(func(t *testing.T, content string) {
		_ = ClassifyPage(Result{Content: content})
	})
}

func FuzzPDF(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("%PDF-1.4\n%%EOF"))
	f.Add([]byte("not a pdf at all"))
	f.Add([]byte("%PDF-1.7\n1 0 obj<<>>endobj\nxref\n0 1\n0000000000 65535 f \ntrailer<<>>\n%%EOF"))
	f.Add([]byte{0x25, 0x50, 0x44, 0x46, 0x00, 0xff, 0xfe, 0xfd})
	if b, err := os.ReadFile("../../testdata/sample.pdf"); err == nil {
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		_, _ = ExtractPDFText(body)
	})
}

func FuzzSitemap(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://example.com/</loc></url></urlset>`))
	f.Add([]byte(`<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>https://example.com/sitemap-1.xml</loc></sitemap></sitemapindex>`))
	f.Add([]byte(`<?xml`))
	f.Add([]byte(`<urlset><url><loc></loc></url></urlset>`))
	if b, err := os.ReadFile("../../testdata/sitemaps/malformed-truncated.xml"); err == nil {
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		root, err := detectXMLRoot(body)
		if err != nil {
			return
		}
		switch root {
		case "sitemapindex":
			var doc sitemapIndexDoc
			_ = xml.Unmarshal(body, &doc)
		case "urlset":
			var doc urlsetDoc
			_ = xml.Unmarshal(body, &doc)
		}
	})
}

func FuzzFeed(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("<?xml"))
	f.Add([]byte(`<rss version="2.0"><channel><title>t</title><item><title>a</title><link>https://example.com</link></item></channel></rss>`))
	f.Add([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>t</title><entry><title>a</title><link href="https://example.com"/></entry></feed>`))
	f.Add([]byte(`{"version":"https://jsonfeed.org/version/1.1","items":[{"id":"1","url":"https://example.com","title":"a"}]}`))
	if b, err := os.ReadFile("../../testdata/feeds/rss-unclosed-cdata.xml"); err == nil {
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		_, _ = parseFeedBytes(body)
	})
}
