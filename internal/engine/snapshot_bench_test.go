package engine

import (
	"os"
	"testing"
)

// BenchmarkSnapshot_Wikipedia drives BuildSnapshot over the 1.3 MB
// wikipedia-latin-phrases.html fixture. Baseline summary lives at
// tests/bench/profiles/BenchmarkSnapshot_Wikipedia.pprof.txt.
func BenchmarkSnapshot_Wikipedia(b *testing.B) {
	htmlBytes, err := os.ReadFile("../../testdata/static/wikipedia-latin-phrases.html")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	htmlStr := string(htmlBytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = BuildSnapshot(htmlStr)
	}
}
