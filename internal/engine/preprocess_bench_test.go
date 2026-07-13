package engine

import (
	"os"
	"testing"
)

func BenchmarkPreprocess_Wikipedia(b *testing.B) {
	htmlBytes, err := os.ReadFile("../../testdata/static/wikipedia-latin-phrases.html")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	htmlStr := string(htmlBytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PreprocessHTML(htmlStr)
	}
}
