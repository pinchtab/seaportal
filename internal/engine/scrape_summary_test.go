package engine

import (
	"strings"
	"testing"
)

func TestSummarizeContentTypesAndRecommendations(t *testing.T) {
	pages := []PageObject{
		{URL: "/a", Status: 200, ContentType: "article", Markdown: strings.Repeat("word ", 100)},
		{URL: "/b", Status: 200, ContentType: "article", Markdown: strings.Repeat("word ", 100)},
		{URL: "/c", Status: 200, ContentType: "product", Markdown: strings.Repeat("word ", 100)},
		{URL: "/d", Status: 500, ContentType: "unknown", Error: "server error"},
		{URL: "/e", Status: 200, ContentType: "page", Markdown: "hi"}, // thin
	}

	sum := summarize(pages, 0, 0)

	if sum.ContentTypes["article"] != 2 {
		t.Errorf("article count = %d, want 2", sum.ContentTypes["article"])
	}
	if sum.ContentTypes["product"] != 1 || sum.ContentTypes["page"] != 1 || sum.ContentTypes["unknown"] != 1 {
		t.Errorf("contentTypes tally wrong: %v", sum.ContentTypes)
	}

	if len(sum.Recommendations) == 0 {
		t.Fatal("expected at least one recommendation")
	}
	joined := strings.Join(sum.Recommendations, " | ")
	if !strings.Contains(joined, "errors or 4xx/5xx") {
		t.Errorf("missing error-rate recommendation: %v", sum.Recommendations)
	}
	if !strings.Contains(joined, "SPA/JS-only") {
		t.Errorf("missing thin/SPA recommendation: %v", sum.Recommendations)
	}
}

func TestSummarizeSamplingRecommendation(t *testing.T) {
	// 500 URLs in sitemap across 3 patterns, only 5 sampled -> recommend sampling.
	pages := make([]PageObject, 5)
	for i := range pages {
		pages[i] = PageObject{Status: 200, ContentType: "article", Markdown: strings.Repeat("w ", 100)}
	}
	sum := summarize(pages, 500, 3)
	joined := strings.Join(sum.Recommendations, " | ")
	if !strings.Contains(joined, "sampling recommended") {
		t.Errorf("expected sampling recommendation, got %v", sum.Recommendations)
	}
}

// regression: ALP-045 — the dense-pattern gate (URLs/pattern >= 10) left
// sparse-clustering, heavily-undersampled sites with no coverage note at all.
func TestSummarizeLowCoverageNote(t *testing.T) {
	okPages := func(n int) []PageObject {
		pages := make([]PageObject, n)
		for i := range pages {
			pages[i] = PageObject{Status: 200, ContentType: "article", Markdown: strings.Repeat("w ", 100)}
		}
		return pages
	}
	coverageNote := func(recs []string) string {
		joined := strings.Join(recs, " | ")
		switch {
		case strings.Contains(joined, "sampling recommended"):
			return "dense"
		case strings.Contains(joined, "coverage"):
			return "low-coverage"
		default:
			return "none"
		}
	}

	// kubernetes.io-shaped: 6453 URLs, 941 patterns (6.9 URLs/pattern), 20 sampled.
	if got := coverageNote(summarize(okPages(20), 6453, 941).Recommendations); got != "low-coverage" {
		t.Errorf("k8s-shaped input: coverage note = %s, want low-coverage", got)
	}

	// smashingmagazine-shaped: 7844 URLs, 31 patterns (253 URLs/pattern), 30 sampled.
	if got := coverageNote(summarize(okPages(30), 7844, 31).Recommendations); got != "dense" {
		t.Errorf("smashing-shaped input: coverage note = %s, want dense", got)
	}

	// Fully sampled sitemap: no coverage note of either kind.
	if got := coverageNote(summarize(okPages(40), 40, 12).Recommendations); got != "none" {
		t.Errorf("fully sampled: coverage note = %s, want none", got)
	}

	// Moderately sampled (40%), sparse patterns: above the 25% floor, no note.
	if got := coverageNote(summarize(okPages(40), 100, 50).Recommendations); got != "none" {
		t.Errorf("40%% coverage sparse: coverage note = %s, want none", got)
	}
}

func TestSummarizeEmptyIsValid(t *testing.T) {
	sum := summarize(nil, 0, 0)
	if sum.ContentTypes == nil {
		t.Error("ContentTypes must be non-nil even for an empty run")
	}
	if len(sum.ContentTypes) != 0 {
		t.Errorf("empty run contentTypes = %v, want empty", sum.ContentTypes)
	}
	if len(sum.Recommendations) != 0 {
		t.Errorf("empty run recommendations = %v, want none", sum.Recommendations)
	}
}
