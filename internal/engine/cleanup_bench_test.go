package engine

import (
	"os"
	"testing"
)

func BenchmarkCleanup_Wikipedia(b *testing.B) {
	htmlBytes, err := os.ReadFile("../../testdata/static/wikipedia-latin-phrases.html")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	md, err := convertHTMLToMarkdown(string(htmlBytes))
	if err != nil {
		b.Fatalf("convert markdown: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CleanupMarkdown(md)
	}
}
