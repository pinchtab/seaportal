package engine

import (
	"os"
	"testing"
)

// BenchmarkCleanup_Wikipedia drives CleanupMarkdown over markdown converted
// from the 1.3 MB wikipedia-latin-phrases.html fixture. HTML->markdown
// conversion is performed once outside the timer.
// Baseline summary lives at
// tests/bench/profiles/BenchmarkCleanup_Wikipedia.pprof.txt.
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
