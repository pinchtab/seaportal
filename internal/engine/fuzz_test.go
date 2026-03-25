package engine

import (
	"testing"
)

func FuzzExtractFromHTML(f *testing.F) {
	f.Add("<html><body><p>Hello world</p></body></html>")
	f.Add("<html><head><title>Test</title></head><body><article><p>Content here</p></article></body></html>")
	f.Add("")
	f.Add("<div>unclosed")
	f.Add("<script>alert('xss')</script><p>safe</p>")
	f.Add("not html at all — just plain text")

	f.Fuzz(func(t *testing.T, html string) {
		// Must not panic
		_, _ = ExtractFromHTML(html, "https://example.com")
	})
}

func FuzzDedupe(f *testing.F) {
	f.Add("Hello world\n\nHello world\n\nSomething else")
	f.Add("")
	f.Add("# Heading\n\nParagraph\n\n# Heading\n\nParagraph")
	f.Add("single line")

	f.Fuzz(func(t *testing.T, content string) {
		result := Dedupe(content)
		if result.DuplicatesFound < 0 {
			t.Fatalf("negative duplicate count: %d", result.DuplicatesFound)
		}
	})
}

func FuzzCleanupMarkdown(f *testing.F) {
	f.Add("# Title\n\nSome **bold** text\n\n---\n\n[link](https://example.com)")
	f.Add("")
	f.Add("   \n\n\n   \n\n")
	f.Add("```code block```")

	f.Fuzz(func(t *testing.T, md string) {
		// Must not panic
		_ = CleanupMarkdown(md)
	})
}

func FuzzBuildSnapshot(f *testing.F) {
	f.Add("<html><body><button>Click me</button><input type='text' placeholder='Name'></body></html>")
	f.Add("<html><body><nav><a href='/'>Home</a></nav></body></html>")
	f.Add("")
	f.Add("<div><div><div><div>deeply nested</div></div></div></div>")

	f.Fuzz(func(t *testing.T, html string) {
		// Must not panic
		_, _ = BuildSnapshot(html)
	})
}
