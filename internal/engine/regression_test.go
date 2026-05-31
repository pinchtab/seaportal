package engine

// Targeted regression fixtures for known failure modes borrowed from competing
// extractors (crawl4ai, reader-mode). Each test asserts on the Markdown body
// surfaced as Result.Content. Fixtures live under testdata/regression/ and use
// only https://example.com URLs — no hostname-specific code paths.

import (
	"os"
	"strings"
	"testing"
)

// regression: empty-element-text-loss (crawl4ai#1966)
func TestRegression_EmptyElementTextLoss(t *testing.T) {
	html, err := os.ReadFile("../../testdata/regression/empty-element-text.html")
	if err != nil {
		t.Fatal(err)
	}
	r := FromHTML(string(html), "https://example.com/regression/empty-element")
	if !strings.Contains(r.Content, "Text after empty span") {
		t.Fatalf("crawl4ai#1966 regressed: text after empty <span> dropped.\nContent:\n%s", r.Content)
	}
}

// regression: heading-skip-level (crawl4ai#1964)
func TestRegression_HeadingHierarchyPreserved(t *testing.T) {
	html, err := os.ReadFile("../../testdata/regression/heading-skip-level.html")
	if err != nil {
		t.Fatal(err)
	}
	r := FromHTML(string(html), "https://example.com/regression/heading-skip")

	// Heading text must survive.
	if !strings.Contains(r.Content, "Top level heading alpha") {
		t.Fatalf("crawl4ai#1964 regressed: h1 text missing.\nContent:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, "Skipped to level three beta") {
		t.Fatalf("crawl4ai#1964 regressed: h3 text missing.\nContent:\n%s", r.Content)
	}

	// Heading levels must be preserved as literal "# " (h1) and "### " (h3)
	// Markdown prefixes — collapse-to-paragraph or level-flattening would
	// drop these substrings.
	if !strings.Contains(r.Content, "# Top level heading alpha") {
		t.Fatalf("crawl4ai#1964 regressed: h1 level lost (no `#` prefix).\nContent:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, "### Skipped to level three beta") {
		t.Fatalf("crawl4ai#1964 regressed: h3 level lost (no `###` prefix).\nContent:\n%s", r.Content)
	}

	// Ordering: h1 must precede h3 in the output.
	h1 := strings.Index(r.Content, "Top level heading alpha")
	h3 := strings.Index(r.Content, "Skipped to level three beta")
	if h1 < 0 || h3 < 0 || h1 >= h3 {
		t.Fatalf("crawl4ai#1964 regressed: heading order not preserved (h1@%d, h3@%d).\nContent:\n%s", h1, h3, r.Content)
	}
}

// regression: trailing-tail-text (crawl4ai#1938)
func TestRegression_TrailingTailText(t *testing.T) {
	html, err := os.ReadFile("../../testdata/regression/trailing-tail-text.html")
	if err != nil {
		t.Fatal(err)
	}
	r := FromHTML(string(html), "https://example.com/regression/trailing-tail")
	if !strings.Contains(r.Content, "trailing text") {
		t.Fatalf("crawl4ai#1938 regressed: tail text after inline <em> dropped.\nContent:\n%s", r.Content)
	}
}

// regression: long-article-4k-truncation (reader#1239)
func TestRegression_LongArticleNotTruncatedAt4k(t *testing.T) {
	html, err := os.ReadFile("../../testdata/regression/long-article-4k-plus.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(html) < 8*1024 {
		t.Fatalf("fixture too small (%d bytes); need >8 KB to defeat a doubled 4k buffer", len(html))
	}
	r := FromHTML(string(html), "https://example.com/regression/long-article")
	if !strings.Contains(r.Content, "FINAL-PARAGRAPH-MARKER") {
		t.Fatalf("reader#1239 regressed: final paragraph missing — content likely truncated.\nContent length: %d\nContent tail:\n%s", len(r.Content), tail(r.Content, 600))
	}
}

// regression: list-and-table-structure (reader#1212, reader#1213)
func TestRegression_ListAndTableStructure(t *testing.T) {
	html, err := os.ReadFile("../../testdata/regression/list-and-table.html")
	if err != nil {
		t.Fatal(err)
	}
	r := FromHTML(string(html), "https://example.com/regression/list-and-table")

	// All three list items must survive.
	for _, item := range []string{"Alpha item one", "Bravo item two", "Charlie item three"} {
		if !strings.Contains(r.Content, item) {
			t.Fatalf("reader#1212 regressed: list item %q missing.\nContent:\n%s", item, r.Content)
		}
	}

	// Table cells must appear, and the Markdown table pipe-row separator
	// must be present — i.e. the table didn't collapse into paragraph text.
	for _, cell := range []string{"HeaderOne", "HeaderTwo", "CellOneA", "CellOneB", "CellTwoA", "CellTwoB"} {
		if !strings.Contains(r.Content, cell) {
			t.Fatalf("reader#1213 regressed: table cell %q missing.\nContent:\n%s", cell, r.Content)
		}
	}
	if !strings.Contains(r.Content, "|") {
		t.Fatalf("reader#1213 regressed: no Markdown table pipe character — table likely flattened.\nContent:\n%s", r.Content)
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
