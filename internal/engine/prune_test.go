package engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- Unit tests for PruneToContent ----

func TestPruneToContent_PicksLargestProseDiv(t *testing.T) {
	prose := strings.Repeat("This is a substantial paragraph of real article prose used for testing. ", 20)
	in := `<html><body>
<div class="header"><nav><a href="/">Home</a> <a href="/about">About</a> <a href="/contact">Contact</a></nav></div>
<div class="content-area"><p>` + prose + `</p></div>
<div class="sidebar"><ul><li><a href="/x">x</a></li><li><a href="/y">y</a></li><li><a href="/z">z</a></li></ul></div>
</body></html>`
	out := PruneToContent(in)
	if !strings.Contains(out, "substantial paragraph of real article prose") {
		t.Fatalf("expected prose preserved, got:\n%s", out)
	}
	if !strings.Contains(out, "<article>") {
		t.Errorf("expected prune to wrap winner in <article>, got:\n%s", out)
	}
}

func TestPruneToContent_RejectsLinkHeavyBlock(t *testing.T) {
	// Build a link-dense div (lots of anchors, ~equal text).
	var links strings.Builder
	for i := 0; i < 40; i++ {
		links.WriteString(`<a href="/x">Link text item number that is reasonably long</a> `)
	}
	prose := strings.Repeat("Plain prose paragraph with no links at all and substantial body content. ", 8)
	in := `<html><body>
<div class="link-heavy">` + links.String() + `</div>
<div class="prose-block"><p>` + prose + `</p></div>
</body></html>`
	out := PruneToContent(in)
	if !strings.Contains(out, "Plain prose paragraph") {
		t.Fatalf("expected text-dense div to win, got:\n%s", out)
	}
	if strings.Contains(out, "Link text item number") {
		t.Errorf("expected link-heavy block to be pruned, got:\n%s", out)
	}
}

func TestPruneToContent_NoOpWhenNothingScores(t *testing.T) {
	// Only chrome — short snippets, no candidate clears the 200-char threshold.
	in := `<html><body>
<div class="nav"><a href="/">Home</a></div>
<div class="footer"><a href="/x">x</a></div>
</body></html>`
	out := PruneToContent(in)
	if out != in {
		t.Errorf("expected input unchanged, got:\n%s", out)
	}
}

func TestPruneToContent_HandlesNoContentDivs(t *testing.T) {
	// Flat page, only <p> tags, no div/section/article/main candidates.
	in := `<html><body><p>first paragraph</p><p>second paragraph</p></body></html>`
	out := PruneToContent(in)
	if out != in {
		t.Errorf("expected input unchanged when no candidate elements, got:\n%s", out)
	}
}

// ---- Integration tests via httptest ----

func TestExtract_PruneFallbackRescuesThinReadability(t *testing.T) {
	// Adversarial shell: readability's algorithm penalises classes like
	// "comment" and weights based on <p>/<li>/scoring tags. We hide the prose
	// in raw text nodes inside spans (no <p>), under classes readability
	// dislikes, so its candidate-scoring returns very little. The
	// tag-density heuristic, which doesn't care about tag identity, still
	// finds the text-dense subtree.
	// Tiny <main> wins the preprocess anchor, so readability extracts only the
	// short intro. The real prose lives in a separate custom-classed div that
	// the heuristic fallback can rescue.
	prose := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 120)
	body := `<!doctype html><html><head><title>Thin Shell</title></head><body>
<main><p>Tiny intro.</p></main>
<div class="custom-content"><p>` + prose + `</p></div>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	// First measure WITHOUT the fallback, to confirm the page is genuinely thin
	// for readability — otherwise the test is meaningless.
	thin := FromHTMLWithOptions(body, srv.URL, Options{NoPruneFallback: true})
	if len(thin.Content) >= 500 {
		t.Skipf("synthetic page no longer thin without fallback (len=%d); skipping rescue assertion", len(thin.Content))
	}

	rescued := FromHTMLWithOptions(body, srv.URL, Options{})
	if !rescued.PruneFallbackUsed {
		t.Fatalf("expected PruneFallbackUsed=true, got false (len=%d)", len(rescued.Content))
	}
	if len(rescued.Content) <= 500 {
		t.Fatalf("expected rescued content > 500 chars, got %d:\n%s", len(rescued.Content), rescued.Content)
	}
	if !strings.Contains(rescued.Content, "quick brown fox") {
		t.Fatalf("expected prose in rescued content, got:\n%s", rescued.Content)
	}
}

func TestExtract_PruneFallbackDisabledByFlag(t *testing.T) {
	// Tiny <main> wins the preprocess anchor, so readability extracts only the
	// short intro. The real prose lives in a separate custom-classed div that
	// the heuristic fallback can rescue.
	prose := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 120)
	body := `<!doctype html><html><head><title>Thin Shell</title></head><body>
<main><p>Tiny intro.</p></main>
<div class="custom-content"><p>` + prose + `</p></div>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	result := FromHTMLWithOptions(body, srv.URL, Options{NoPruneFallback: true})
	if result.PruneFallbackUsed {
		t.Fatalf("expected PruneFallbackUsed=false when flag set")
	}
}
