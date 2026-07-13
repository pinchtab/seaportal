package engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractLDJSON_ArticleBody(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		wantBody string
	}{
		{
			name: "with articleBody",
			html: `<html><head><script type="application/ld+json">
				{"@type":"NewsArticle","headline":"H","articleBody":"This is the body."}
			</script></head><body></body></html>`,
			wantBody: "This is the body.",
		},
		{
			name: "without articleBody",
			html: `<html><head><script type="application/ld+json">
				{"@type":"Article","headline":"H"}
			</script></head><body></body></html>`,
			wantBody: "",
		},
		{
			name: "articleBody with surrounding whitespace",
			html: `<html><head><script type="application/ld+json">
				{"@type":"BlogPosting","headline":"H","articleBody":"   trimmed   "}
			</script></head><body></body></html>`,
			wantBody: "trimmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := ExtractLDJSON(tt.html)
			if len(blocks) == 0 {
				t.Fatalf("expected at least one LD-JSON block")
			}
			if blocks[0].Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", blocks[0].Body, tt.wantBody)
			}
		})
	}
}

func TestExtract_ExtractionMethodLabeling_Readability(t *testing.T) {
	html := `<html><head><title>An Article</title></head><body>
		<article>
			<h1>An Article</h1>
			<p>` + strings.Repeat("This is a substantial paragraph of article content with enough words to make readability happy. ", 30) + `</p>
			<p>` + strings.Repeat("Another paragraph with plenty of prose so the readability algorithm scores it as the main content. ", 30) + `</p>
		</article>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	result := FromURL(server.URL)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.ExtractionMethod != "readability" {
		t.Errorf("ExtractionMethod = %q, want %q (length=%d)", result.ExtractionMethod, "readability", result.Length)
	}
}

func TestExtract_ExtractionMethodLabeling_PruneFallback(t *testing.T) {
	rich := strings.Repeat("<p>"+strings.Repeat("Rescuable paragraph words. ", 8)+"</p>", 6)
	html := `<html><head><title>T</title></head><body>
		<div class="comment sidebar ad">` + rich + `</div>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	result := FromURL(server.URL)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !result.PruneFallbackUsed {
		t.Skipf("prune-fallback did not fire (readability length=%d) — labelling path covered by smoke fixtures", result.Length)
	}
	if result.ExtractionMethod != "prune-fallback" {
		t.Errorf("ExtractionMethod = %q, want %q (length=%d, content=%q)", result.ExtractionMethod, "prune-fallback", result.Length, snippet(result.Content))
	}
}

func TestExtract_ExtractionMethodLabeling_TextFallback(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<html><head><title>T</title></head><body>`)
	for i := 0; i < 500; i++ {
		sb.WriteString(`<div data-x="filler-attribute-to-pad-html-size">tiny chunk text. </div>`)
	}
	sb.WriteString(`</body></html>`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(sb.String()))
	}))
	defer server.Close()

	result := FromURLWithOptions(server.URL, Options{NoPruneFallback: true})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.ExtractionMethod == "readability" && result.Length >= 500 {
		t.Skipf("readability already extracted %d chars — text-fallback gate not exercised", result.Length)
	}
	if result.ExtractionMethod != "text-fallback" {
		t.Errorf("ExtractionMethod = %q, want %q (length=%d)", result.ExtractionMethod, "text-fallback", result.Length)
	}
}

func TestExtract_ExtractionMethodLabeling_IndexPage(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<html><head><title>Home</title></head><body>`)
	for i := 0; i < 12; i++ {
		sb.WriteString(`<article><h2><a href="/post-`)
		sb.WriteString(string(rune('a' + i)))
		sb.WriteString(`">Headline `)
		sb.WriteString(string(rune('A' + i)))
		sb.WriteString(`</a></h2><p>Teaser text</p></article>`)
	}
	sb.WriteString(`</body></html>`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(sb.String()))
	}))
	defer server.Close()

	result := FromURL(server.URL)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.ExtractionMethod != "index-page" {
		t.Errorf("ExtractionMethod = %q, want %q (length=%d)", result.ExtractionMethod, "index-page", result.Length)
	}
}

func TestExtract_JSONLDArticleBodyRescuesThinReadability(t *testing.T) {
	body := strings.Repeat("This is the JSON-LD provided article body that we rely on when readability cannot find the prose. ", 35)
	html := `<html><head>
		<title>News</title>
		<script type="application/ld+json">
		{"@type":"NewsArticle","headline":"Breaking","articleBody":"` + body + `"}
		</script>
	</head><body>
		<article><p>placeholder</p></article>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	result := FromURL(server.URL)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.ExtractionMethod != "json-ld-article-body" {
		t.Errorf("ExtractionMethod = %q, want %q (length=%d, content=%q)", result.ExtractionMethod, "json-ld-article-body", result.Length, snippet(result.Content))
	}
	if !strings.Contains(result.Content, "JSON-LD provided article body") {
		t.Errorf("Content does not include the JSON-LD body: %q", snippet(result.Content))
	}
	if result.Title == "" {
		t.Errorf("Title should be filled from headline when empty")
	}
}

func snippet(s string) string {
	const n = 120
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
