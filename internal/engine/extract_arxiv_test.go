package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtract_ArxivAuthors(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "ssr", "arxiv-attention.html")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	result := FromHTML(string(raw), "https://arxiv.org/abs/1706.03762")

	if !strings.Contains(result.Byline, "Vaswani") {
		t.Errorf("Byline should contain 'Vaswani', got %q", result.Byline)
	}
	otherSurnames := []string{"Shazeer", "Parmar", "Uszkoreit", "Jones", "Gomez", "Kaiser", "Polosukhin"}
	found := 0
	for _, s := range otherSurnames {
		if strings.Contains(result.Byline, s) {
			found++
		}
	}
	if found < 2 {
		t.Errorf("Byline should contain at least 2 other arXiv-paper surnames, got %d in %q", found, result.Byline)
	}

	if !strings.Contains(result.Content, "**Authors:**") {
		t.Errorf("Content should contain '**Authors:**' line, got prefix: %q", contentHead(result.Content, 200))
	}
	if !strings.HasPrefix(strings.TrimLeft(result.Content, " \t\n"), "**Authors:**") {
		t.Errorf("Content should START with '**Authors:**' line, got: %q", contentHead(result.Content, 200))
	}
	if !strings.Contains(result.Content, "Vaswani") {
		t.Errorf("Content should mention 'Vaswani', got prefix: %q", contentHead(result.Content, 400))
	}

	if !strings.Contains(result.Title, "Attention") {
		t.Errorf("Title should contain 'Attention', got %q", result.Title)
	}
}

func TestExtract_NoCitationAuthorsNoOp(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head><title>Plain Page</title></head>
<body>
<article>
<h1>Heading</h1>
<p>This is body content with no author metadata at all.</p>
<p>Another paragraph so readability is happy.</p>
</article>
</body>
</html>`

	result := FromHTML(html, "https://example.com/plain")

	if strings.Contains(result.Content, "**Authors:**") {
		t.Errorf("Content must not contain '**Authors:**' for non-academic pages, got: %q", contentHead(result.Content, 200))
	}
	if strings.Contains(result.Byline, ";") && strings.Count(result.Byline, ";") >= 2 {
		t.Errorf("Byline should not look like a joined author list, got %q", result.Byline)
	}
}

func TestExtractMetaAuthors_OrderAndDedupe(t *testing.T) {
	html := `
<meta name="citation_author" content="Vaswani, Ashish" />
<meta name="citation_author" content="Shazeer, Noam" />
<meta name="DC.creator" content="Vaswani, Ashish" />
<meta name="dc.creator" content="Parmar, Niki"/>
<meta name="citation_author" content="" />
<meta name="citation_author" content="  Jones, Llion  " />
`
	got := ExtractMetaAuthors(html)
	want := []string{"Vaswani, Ashish", "Shazeer, Noam", "Parmar, Niki", "Jones, Llion"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] got %q want %q", i, got[i], w)
		}
	}
}

func contentHead(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
