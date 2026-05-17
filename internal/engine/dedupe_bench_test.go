package engine

import (
	"os"
	"testing"
)

// BenchmarkDedupe_Wikipedia drives Dedupe over markdown converted from the
// 1.3 MB wikipedia-latin-phrases.html fixture. HTML->markdown conversion +
// cleanup are performed once outside the timer so the bench reflects only
// dedupe cost.
// Baseline summary lives at
// tests/bench/profiles/BenchmarkDedupe_Wikipedia.pprof.txt.
func BenchmarkDedupe_Wikipedia(b *testing.B) {
	htmlBytes, err := os.ReadFile("../../testdata/static/wikipedia-latin-phrases.html")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	md, err := convertHTMLToMarkdown(string(htmlBytes))
	if err != nil {
		b.Fatalf("convert markdown: %v", err)
	}
	md = CleanupMarkdown(md)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Dedupe(md)
	}
}
