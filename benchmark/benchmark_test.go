package benchmark

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pinchtab/seaportal/pkg/portal"
)

var testdataDir = filepath.Join("..", "testdata")

func loadFixture(t testing.TB, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir, path))
	if err != nil {
		t.Fatalf("failed to load fixture %s: %v", path, err)
	}
	return string(data)
}

func BenchmarkExtractFromHTML_Simple(b *testing.B) {
	html := loadFixture(b, "static/simple.html")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = portal.ExtractFromHTML(html, "https://example.com")
	}
}

func BenchmarkExtractFromHTML_Article(b *testing.B) {
	html := loadFixture(b, "static/article.html")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = portal.ExtractFromHTML(html, "https://example.com")
	}
}

func BenchmarkDedupe(b *testing.B) {
	block := "This is a sufficiently long block of text that should be detected as a duplicate when it appears multiple times in the content."
	content := ""
	for i := 0; i < 50; i++ {
		if i%3 == 0 {
			content += block + "\n\n"
		} else {
			content += "Unique block number " + string(rune('A'+i%26)) + " with enough content to be meaningful.\n\n"
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = portal.Dedupe(content)
	}
}

func BenchmarkCleanupMarkdown(b *testing.B) {
	md := "# Title\n\nSome **bold** and *italic* text.\n\n"
	content := ""
	for i := 0; i < 100; i++ {
		content += md
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = portal.CleanupMarkdown(content)
	}
}

func BenchmarkClassifyPage(b *testing.B) {
	html := loadFixture(b, "static/simple.html")
	result := portal.Result{
		Content:    html,
		Confidence: 85,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = portal.ClassifyPage(result)
	}
}

func BenchmarkBuildSnapshot_Simple(b *testing.B) {
	html := loadFixture(b, "static/simple.html")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = portal.BuildSnapshot(html)
	}
}

func BenchmarkBuildSnapshot_Complex(b *testing.B) {
	html := loadFixture(b, "static/table.html")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = portal.BuildSnapshot(html)
	}
}
