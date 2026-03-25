package integration_test

import (
	"strings"
	"testing"

	"github.com/pinchtab/seaportal/pkg/portal"
)

func TestPublicAPI_ExtractFromHTML(t *testing.T) {
	html := `<html><head><title>Test Page</title></head>
		<body><article><h1>Hello</h1><p>This is test content for extraction.</p></article></body></html>`

	md, err := portal.ExtractFromHTML(html, "https://example.com")
	if err != nil {
		t.Fatalf("ExtractFromHTML failed: %v", err)
	}
	if md == "" {
		t.Fatal("expected non-empty markdown output")
	}
}

func TestPublicAPI_Dedupe(t *testing.T) {
	// Blocks must exceed MinBlockLen (default 20) to be tracked for dedup
	block := "This is a sufficiently long block of text for deduplication testing purposes."
	input := block + "\n\nSomething unique here.\n\n" + block + "\n\nAnother unique block."
	result := portal.Dedupe(input)

	if result.DuplicatesFound < 1 {
		t.Fatalf("expected at least 1 duplicate, got %d", result.DuplicatesFound)
	}
	if result.Content == "" {
		t.Fatal("expected non-empty deduplicated content")
	}
}

func TestPublicAPI_ClassifyPage(t *testing.T) {
	result := portal.Result{
		Content:    "Some extracted content here for testing",
		Confidence: 90,
	}
	profile := portal.ClassifyPage(result)
	s := profile.String()
	if s == "" {
		t.Fatal("expected non-empty profile string")
	}
}

func TestPublicAPI_CleanupMarkdown(t *testing.T) {
	input := "# Title\n\n\n\n\nToo many blanks\n\n\n\nEnd"
	result := portal.CleanupMarkdown(input)
	if strings.Contains(result, "\n\n\n\n") {
		t.Fatal("expected consecutive blank lines to be collapsed")
	}
}

func TestPublicAPI_BuildSnapshot(t *testing.T) {
	html := `<html><body>
		<nav><a href="/">Home</a><a href="/about">About</a></nav>
		<main><h1>Page Title</h1><p>Content</p></main>
		<button>Click me</button>
		<input type="text" placeholder="Search">
	</body></html>`

	tree, err := portal.BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}
	if tree == nil {
		t.Fatal("expected non-nil snapshot tree")
	}
}

func TestPublicAPI_BuildSnapshotWithOptions(t *testing.T) {
	html := `<html><body>
		<button>Click</button>
		<input type="text">
		<a href="/">Link</a>
		<p>Not interactive</p>
	</body></html>`

	opts := portal.SnapshotOptions{
		FilterInteractive: true,
	}
	tree, err := portal.BuildSnapshotWithOptions(html, opts)
	if err != nil {
		t.Fatalf("BuildSnapshotWithOptions failed: %v", err)
	}
	if tree == nil {
		t.Fatal("expected non-nil snapshot tree")
	}
}
