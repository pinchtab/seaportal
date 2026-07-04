package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleScrapeResult() *ScrapeResult {
	return &ScrapeResult{
		Site: SiteInfo{
			BaseURL:            "https://ex.com",
			Title:              "Example Site",
			SitemapFound:       true,
			TotalURLsInSitemap: 1200,
			SampledPages:       3,
		},
		PageGroups: []PageGroup{
			{Pattern: "/blog/*", TotalInSitemap: 800, Sampled: 2},
			{Pattern: "/products/*/detail", TotalInSitemap: 400, Sampled: 1},
		},
		Pages: []PageObject{
			{URL: "https://ex.com/", Title: "Home", Status: 200, ContentType: "page", Markdown: "# Home\n\nWelcome."},
			{URL: "https://ex.com/blog/first-post", Title: "First Post", Status: 200, ContentType: "article", Markdown: "# First Post\n\nBody."},
			{URL: "https://ex.com/blog/first-post", Title: "Dup Slug", Status: 200, ContentType: "article", Markdown: "# Dup\n\nBody."},
		},
		Summary: ScrapeSummary{
			ContentTypes:    map[string]int{"article": 2, "page": 1},
			Recommendations: []string{"sitemap lists 1200 URLs; sampling recommended"},
		},
	}
}

func TestRenderScrapeJSON(t *testing.T) {
	res := sampleScrapeResult()
	data, err := RenderScrapeJSON(res)
	if err != nil {
		t.Fatalf("RenderScrapeJSON: %v", err)
	}
	var round ScrapeResult
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if round.Site.BaseURL != "https://ex.com" || len(round.Pages) != 3 {
		t.Errorf("round-trip mismatch: %+v", round.Site)
	}
	// Spec field names present.
	for _, key := range []string{`"baseURL"`, `"pageGroups"`, `"totalInSitemap"`, `"contentTypes"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("json missing spec key %s", key)
		}
	}
}

func TestRenderScrapeMarkdown(t *testing.T) {
	md := RenderScrapeMarkdown(sampleScrapeResult())
	for _, want := range []string{"# Example Site", "## Page groups", "`/blog/*`", "## Pages", "### First Post", "## Summary", "article: 2"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q\n---\n%s", want, md)
		}
	}
}

func TestWriteScrapeDirectory(t *testing.T) {
	dir := t.TempDir()
	rel, err := WriteScrapeDirectory(sampleScrapeResult(), dir)
	if err != nil {
		t.Fatalf("WriteScrapeDirectory: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "result.json")); err != nil {
		t.Errorf("result.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.md")); err != nil {
		t.Errorf("index.md manifest missing: %v", err)
	}
	if len(rel) != 3 {
		t.Fatalf("got %d page files, want 3", len(rel))
	}
	// Slugs are unique even for the duplicate URL.
	seen := map[string]bool{}
	for _, r := range rel {
		if seen[r] {
			t.Errorf("duplicate slug path %s", r)
		}
		seen[r] = true
		if _, err := os.Stat(filepath.Join(dir, r)); err != nil {
			t.Errorf("page file %s missing: %v", r, err)
		}
	}
}
