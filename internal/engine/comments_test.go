package engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

func parseFirstElement(t *testing.T, htmlStr string) *xhtml.Node {
	t.Helper()
	doc, err := xhtml.Parse(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Find first element inside <body> that is not a text node.
	var find func(n *xhtml.Node) *xhtml.Node
	find = func(n *xhtml.Node) *xhtml.Node {
		if n == nil {
			return nil
		}
		if n.Type == xhtml.ElementNode && n.Data == "body" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == xhtml.ElementNode {
					return c
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if got := find(c); got != nil {
				return got
			}
		}
		return nil
	}
	return find(doc)
}

func TestDetectCommentContainer_DisqusID(t *testing.T) {
	n := parseFirstElement(t, `<div id="disqus_thread"></div>`)
	if !detectCommentContainer(n) {
		t.Fatal("expected disqus_thread id to be detected")
	}
}

func TestDetectCommentContainer_CommentsClass(t *testing.T) {
	n := parseFirstElement(t, `<section class="post-comments comments"></section>`)
	if !detectCommentContainer(n) {
		t.Fatal("expected class containing 'comments' token to be detected")
	}
}

func TestDetectCommentContainer_RoleRegionAriaLabel(t *testing.T) {
	n := parseFirstElement(t, `<div role="region" aria-label="Reader Comments"></div>`)
	if !detectCommentContainer(n) {
		t.Fatal("expected role=region with comment aria-label to be detected")
	}
}

func TestDetectCommentContainer_DataComponent(t *testing.T) {
	n := parseFirstElement(t, `<div data-component="comments"></div>`)
	if !detectCommentContainer(n) {
		t.Fatal("expected data-component=comments to be detected")
	}
}

func TestDetectCommentContainer_NegativeArticle(t *testing.T) {
	n := parseFirstElement(t, `<article class="post-body"><p>Some article that mentions comments in prose.</p></article>`)
	if detectCommentContainer(n) {
		t.Fatal("article with prose mentioning comments should NOT be detected")
	}
}

func TestDetectCommentContainer_NegativeDocsPage(t *testing.T) {
	// A docs page about "JS comments" — header text contains the word but
	// the container element attributes do not.
	n := parseFirstElement(t, `<section class="docs-chapter"><h2>JS comments</h2><p>About // and /* */ syntax.</p></section>`)
	if detectCommentContainer(n) {
		t.Fatal("docs page with 'comments' in prose should NOT be detected")
	}
}

func TestStripCommentContainers_RemovesDisqus(t *testing.T) {
	html := `<html><body><main><article><p>` + strings.Repeat("Real article content here. ", 30) + `</p></article><section id="disqus_thread"><div>spam comment text</div></section></main></body></html>`
	out := stripCommentContainers(html)
	if strings.Contains(out, "disqus_thread") {
		t.Fatal("disqus_thread container should be removed")
	}
	if strings.Contains(out, "spam comment text") {
		t.Fatal("disqus comment content should be removed")
	}
	if !strings.Contains(out, "Real article content here") {
		t.Fatal("article body should be preserved")
	}
}

func TestStripCommentContainers_LeavesArticleAlone(t *testing.T) {
	html := `<html><body><main><article class="post"><p>` + strings.Repeat("Pure article prose. ", 30) + `</p></article></main></body></html>`
	out := stripCommentContainers(html)
	if !strings.Contains(out, "Pure article prose") {
		t.Fatal("article without comment container should be unchanged")
	}
}

func TestStripCommentContainers_BodyEmptinessGuard(t *testing.T) {
	// The only substantive content is wrapped by a comments container —
	// removing it would empty the body. Strip should abort.
	html := `<html><body><div class="page"><section id="disqus_thread"><p>` + strings.Repeat("Body wrapped in comments widget. ", 20) + `</p></section></div></body></html>`
	out := stripCommentContainers(html)
	if !strings.Contains(out, "Body wrapped in comments widget") {
		t.Fatal("body-emptiness guard should preserve content when strip would empty body")
	}
}

func TestExtractComments_BasicAuthorTextTimestamp(t *testing.T) {
	html := `<html><body><main><article><p>` + strings.Repeat("Article. ", 40) + `</p></article>` +
		`<section id="disqus_thread">` +
		`<li class="comment"><span class="author">Alice</span><time datetime="2024-01-02T03:04:05Z">Jan 2</time><div class="comment-body">First comment text.</div></li>` +
		`<li class="comment"><span class="author">Bob</span><time datetime="2024-02-03T04:05:06Z">Feb 3</time><div class="comment-body">Second comment text.</div></li>` +
		`</section></main></body></html>`
	comments := ExtractComments(html, "https://example.com")
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	if comments[0].Author != "Alice" || comments[0].Timestamp != "2024-01-02T03:04:05Z" || comments[0].Text != "First comment text." {
		t.Fatalf("first comment mismatch: %+v", comments[0])
	}
	if comments[1].Author != "Bob" || comments[1].Timestamp != "2024-02-03T04:05:06Z" || comments[1].Text != "Second comment text." {
		t.Fatalf("second comment mismatch: %+v", comments[1])
	}
}

func TestExtractComments_HandlesMissingAuthor(t *testing.T) {
	html := `<html><body><main><article><p>` + strings.Repeat("Article. ", 40) + `</p></article>` +
		`<section id="disqus_thread"><li class="comment"><time datetime="2024-01-02">x</time><div class="comment-body">Just text.</div></li></section>` +
		`</main></body></html>`
	c := ExtractComments(html, "")
	if len(c) != 1 {
		t.Fatalf("want 1, got %d", len(c))
	}
	if c[0].Author != "" {
		t.Fatalf("expected empty author, got %q", c[0].Author)
	}
	if c[0].Text != "Just text." {
		t.Fatalf("text mismatch: %q", c[0].Text)
	}
}

func TestExtractComments_HandlesMissingTimestamp(t *testing.T) {
	html := `<html><body><main><article><p>` + strings.Repeat("Article. ", 40) + `</p></article>` +
		`<section id="disqus_thread"><li class="comment"><span class="author">Carol</span><div class="comment-body">No time here.</div></li></section>` +
		`</main></body></html>`
	c := ExtractComments(html, "")
	if len(c) != 1 {
		t.Fatalf("want 1, got %d", len(c))
	}
	if c[0].Timestamp != "" {
		t.Fatalf("expected empty timestamp, got %q", c[0].Timestamp)
	}
	if c[0].Author != "Carol" || c[0].Text != "No time here." {
		t.Fatalf("mismatch: %+v", c[0])
	}
}

func TestExtractComments_WalksMultipleContainers(t *testing.T) {
	html := `<html><body><main><article><p>` + strings.Repeat("Article. ", 40) + `</p></article>` +
		`<section id="disqus_thread"><li class="comment"><div class="comment-body">A</div></li></section>` +
		`<section class="comments"><li class="comment"><div class="comment-body">B</div></li></section>` +
		`</main></body></html>`
	c := ExtractComments(html, "")
	if len(c) != 2 {
		t.Fatalf("want 2 comments across containers, got %d", len(c))
	}
	if c[0].Text != "A" || c[1].Text != "B" {
		t.Fatalf("mismatch: %+v", c)
	}
}

// Integration tests via httptest.

func commentPageHTML() string {
	return `<!doctype html><html><head><title>Test Article</title></head><body><main><article>` +
		`<h1>Test Article</h1>` +
		`<p>` + strings.Repeat("This is the main article body content. ", 30) + `</p>` +
		`</article>` +
		`<section id="disqus_thread">` +
		`<li class="comment"><span class="author">Alice</span><time datetime="2024-01-02T00:00:00Z">Jan 2</time><div class="comment-body">First reader comment.</div></li>` +
		`<li class="comment"><span class="author">Bob</span><time datetime="2024-01-03T00:00:00Z">Jan 3</time><div class="comment-body">Second reader comment.</div></li>` +
		`<li class="comment"><span class="author">Carol</span><time datetime="2024-01-04T00:00:00Z">Jan 4</time><div class="comment-body">Third reader comment.</div></li>` +
		`</section>` +
		`</main></body></html>`
}

func TestExtract_WithCommentsFlagOn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(commentPageHTML()))
	}))
	defer srv.Close()

	res := FromURLWithOptions(srv.URL, Options{WithComments: true})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if strings.Contains(res.Content, "First reader comment") ||
		strings.Contains(res.Content, "Second reader comment") ||
		strings.Contains(res.Content, "Third reader comment") {
		t.Fatalf("comments should be stripped from Content; got: %q", res.Content)
	}
	if len(res.Comments) != 3 {
		t.Fatalf("want 3 comments, got %d (%+v)", len(res.Comments), res.Comments)
	}
	for i, want := range []struct{ author, text string }{
		{"Alice", "First reader comment."},
		{"Bob", "Second reader comment."},
		{"Carol", "Third reader comment."},
	} {
		if res.Comments[i].Author != want.author {
			t.Errorf("comment[%d] author: want %q got %q", i, want.author, res.Comments[i].Author)
		}
		if res.Comments[i].Text != want.text {
			t.Errorf("comment[%d] text: want %q got %q", i, want.text, res.Comments[i].Text)
		}
		if res.Comments[i].Timestamp == "" {
			t.Errorf("comment[%d] missing timestamp", i)
		}
	}
}

func TestExtract_WithCommentsFlagOffDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(commentPageHTML()))
	}))
	defer srv.Close()

	res := FromURLWithOptions(srv.URL, Options{})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if strings.Contains(res.Content, "First reader comment") ||
		strings.Contains(res.Content, "Second reader comment") ||
		strings.Contains(res.Content, "Third reader comment") {
		t.Fatalf("comments should be stripped from Content even with flag off; got: %q", res.Content)
	}
	if res.Comments != nil {
		t.Fatalf("expected res.Comments == nil with flag off, got %+v", res.Comments)
	}
}
