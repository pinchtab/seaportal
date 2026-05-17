package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractLDJSON_Language(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{
			name: "string",
			html: `<script type="application/ld+json">{"@type":"NewsArticle","headline":"H","inLanguage":"en-US"}</script>`,
			want: "en-US",
		},
		{
			name: "named object",
			html: `<script type="application/ld+json">{"@type":"NewsArticle","headline":"H","inLanguage":{"@type":"Language","name":"English"}}</script>`,
			want: "English",
		},
		{
			name: "missing",
			html: `<script type="application/ld+json">{"@type":"NewsArticle","headline":"H"}</script>`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks := ExtractLDJSON(tc.html)
			if len(blocks) == 0 {
				t.Fatalf("no blocks extracted")
			}
			if blocks[0].Language != tc.want {
				t.Errorf("Language = %q, want %q", blocks[0].Language, tc.want)
			}
		})
	}
}

func TestExtractLDJSON_Section(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{
			name: "string",
			html: `<script type="application/ld+json">{"@type":"NewsArticle","headline":"H","articleSection":"Technology"}</script>`,
			want: "Technology",
		},
		{
			name: "array takes first",
			html: `<script type="application/ld+json">{"@type":"NewsArticle","headline":"H","articleSection":["Technology","Science"]}</script>`,
			want: "Technology",
		},
		{
			name: "missing",
			html: `<script type="application/ld+json">{"@type":"NewsArticle","headline":"H"}</script>`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks := ExtractLDJSON(tc.html)
			if len(blocks) == 0 {
				t.Fatalf("no blocks extracted")
			}
			if blocks[0].Section != tc.want {
				t.Errorf("Section = %q, want %q", blocks[0].Section, tc.want)
			}
		})
	}
}

func TestApplyLDJSONMetadata_PopulatesEmptyResult(t *testing.T) {
	r := &Result{}
	blocks := []LDJSONBlock{{
		Headline: "Demo",
		Author:   "Jane Doe",
		DatePub:  "2024-03-15T10:00:00Z",
		Language: "en-US",
		Section:  "Technology",
	}}
	applyLDJSONMetadata(r, blocks)
	if r.Byline != "Jane Doe" {
		t.Errorf("Byline = %q, want Jane Doe", r.Byline)
	}
	if r.PublishedDate != "2024-03-15T10:00:00Z" {
		t.Errorf("PublishedDate = %q", r.PublishedDate)
	}
	if r.Language != "en-US" {
		t.Errorf("Language = %q", r.Language)
	}
	if r.Section != "Technology" {
		t.Errorf("Section = %q", r.Section)
	}
}

func TestApplyLDJSONMetadata_OverridesReadabilityByline(t *testing.T) {
	r := &Result{Byline: "Submitted on 2024-03-15"}
	blocks := []LDJSONBlock{{
		Headline: "Demo",
		Author:   "Jane Doe",
	}}
	applyLDJSONMetadata(r, blocks)
	if r.Byline != "Jane Doe" {
		t.Errorf("Byline should be overridden by JSON-LD, got %q", r.Byline)
	}
}

func TestApplyLDJSONMetadata_SkipsNonArticleBlocks(t *testing.T) {
	r := &Result{Byline: "original"}
	// BreadcrumbList / WebSite blocks: no Headline.
	blocks := []LDJSONBlock{
		{Type: "BreadcrumbList"},
		{Type: "WebSite", URL: "https://example.com"},
	}
	applyLDJSONMetadata(r, blocks)
	if r.Byline != "original" {
		t.Errorf("Byline should be untouched, got %q", r.Byline)
	}
	if r.PublishedDate != "" || r.Language != "" || r.Section != "" {
		t.Errorf("non-Article blocks should not populate fields")
	}
}

func TestExtract_LDJSONFixture(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "static", "article-ldjson.html")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	result := FromHTML(string(raw), "https://example.com/article")
	if result.Byline != "Jane Doe" {
		t.Errorf("Byline = %q, want Jane Doe", result.Byline)
	}
	if result.PublishedDate != "2024-03-15T10:00:00Z" {
		t.Errorf("PublishedDate = %q", result.PublishedDate)
	}
	if result.Language != "en-US" {
		t.Errorf("Language = %q", result.Language)
	}
	if result.Section != "Technology" {
		t.Errorf("Section = %q", result.Section)
	}
}
