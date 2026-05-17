package engine

import (
	"os"
	"testing"
)

// BenchmarkFromHTML_WikipediaLatinPhrases drives the full extraction pipeline
// against the 1.3 MB wikipedia-latin-phrases.html fixture — the p99 outlier in
// the latency budget gate. Used to identify and verify fixes for quadratic
// hot paths.
func BenchmarkFromHTML_WikipediaLatinPhrases(b *testing.B) {
	html, err := os.ReadFile("../../testdata/static/wikipedia-latin-phrases.html")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	url := "https://en.wikipedia.org/wiki/List_of_Latin_phrases_(full)"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FromHTML(string(html), url)
	}
}
