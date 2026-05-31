package engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTruncate_BelowBudgetNoOp(t *testing.T) {
	md := "short body"
	out, did := TruncateMarkdownAtParagraph(md, 100)
	if did {
		t.Fatalf("expected no truncation, got didTrunc=true")
	}
	if out != md {
		t.Fatalf("expected unchanged, got %q", out)
	}
}

func TestTruncate_AboveBudgetCutsAtParagraph(t *testing.T) {
	// budget = 15 * 4 = 60 chars.
	p1 := "paragraph one." // 14
	p2 := "paragraph two." // 14
	p3 := strings.Repeat("x", 200)
	md := p1 + "\n\n" + p2 + "\n\n" + p3
	out, did := TruncateMarkdownAtParagraph(md, 15)
	if !did {
		t.Fatalf("expected truncation")
	}
	// Last "\n\n" within md[:60] is the one between p2 and p3 (starts at offset 30,
	// fits inside 60). Cut at 30 → keep p1+\n\n+p2.
	if !strings.HasPrefix(out, p1+"\n\n"+p2+"\n\n*[truncated]*\n") {
		t.Fatalf("expected output to start with p1\\n\\np2 then marker, got %q", out)
	}
	if strings.Contains(out, "x") {
		t.Fatalf("expected truncation before xxx body, got %q", out)
	}
}

func TestTruncate_NoParagraphBoundaryFallsBackToLine(t *testing.T) {
	// budget = 5 * 4 = 20 chars; no \n\n, only \n.
	md := "aaaaaa\nbbbbbb\ncccccc\ndddddd\neeeeee"
	out, did := TruncateMarkdownAtParagraph(md, 5)
	if !did {
		t.Fatalf("expected truncation")
	}
	// LastIndex of "\n" within first 20 bytes: "aaaaaa\nbbbbbb\ncccccc" → last \n at 13.
	if !strings.HasPrefix(out, "aaaaaa\nbbbbbb") {
		t.Fatalf("expected cut at line boundary, got %q", out)
	}
	if strings.Contains(out[:len(out)-len("\n\n*[truncated]*\n")], "cccccc") {
		t.Fatalf("expected truncation before cccccc, got %q", out)
	}
}

func TestTruncate_NoBoundaryAtAllHardCuts(t *testing.T) {
	// budget = 5 * 4 = 20 chars; no newlines at all.
	md := strings.Repeat("x", 200)
	out, did := TruncateMarkdownAtParagraph(md, 5)
	if !did {
		t.Fatalf("expected truncation")
	}
	body := strings.TrimSuffix(out, "\n\n*[truncated]*\n")
	if len(body) != 20 {
		t.Fatalf("expected hard cut at 20 chars, got %d (%q)", len(body), body)
	}
}

func TestTruncate_AppendsTruncatedMarker(t *testing.T) {
	md := strings.Repeat("hello world.\n\n", 50)
	out, did := TruncateMarkdownAtParagraph(md, 5)
	if !did {
		t.Fatalf("expected truncation")
	}
	if !strings.HasSuffix(out, "*[truncated]*\n") {
		t.Fatalf("expected suffix marker, got %q", out)
	}
}

func TestTruncate_ZeroBudgetNoOp(t *testing.T) {
	md := strings.Repeat("x", 1000)
	out, did := TruncateMarkdownAtParagraph(md, 0)
	if did {
		t.Fatalf("expected no truncation for zero budget")
	}
	if out != md {
		t.Fatalf("expected unchanged")
	}
}

const truncIntegrationHTML = `<!DOCTYPE html>
<html><head><title>Trunc Test</title></head>
<body>
<article>
<h1>Trunc Test</h1>
<p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
</article>
</body></html>`

const longParagraph = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat."

func TestExtract_MaxTokensTruncatesMarkdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(truncIntegrationHTML))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL, Options{MaxTokens: 50})
	if !result.Truncated {
		t.Fatalf("expected result.Truncated=true, content length=%d", result.Length)
	}
	if len(result.Content) >= 250 {
		t.Fatalf("expected truncated content < 250 chars, got %d", len(result.Content))
	}
	if !strings.HasSuffix(result.Content, "*[truncated]*\n") {
		t.Fatalf("expected content to end with marker, got tail=%q", tailOf(result.Content, 40))
	}
	if result.Length != len(result.Content) {
		t.Fatalf("expected Length==len(Content), got %d vs %d", result.Length, len(result.Content))
	}
}

func TestExtract_MaxTokensDefaultNoTruncation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(truncIntegrationHTML))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL, Options{})
	if result.Truncated {
		t.Fatalf("expected Truncated=false with default options")
	}
	if strings.HasSuffix(result.Content, "*[truncated]*\n") {
		t.Fatalf("default output should not have truncated marker")
	}
}

func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
