package engine

import (
	"os"
	"testing"
)

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
