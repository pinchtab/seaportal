package engine

import (
	"strings"
	"testing"
)

func TestExtract_WikipediaLatinPhrases(t *testing.T) {
	skipHeavyFixture(t)
	data := loadFixture(t, "static/wikipedia-latin-phrases.html")

	result := FromHTML(data, "https://en.wikipedia.org/wiki/List_of_Latin_phrases_(full)")
	if result.Error != "" {
		t.Fatalf("extraction error: %s", result.Error)
	}
	if result.Length < 20000 {
		t.Errorf("expected length > 20000, got %d", result.Length)
	}

	lower := strings.ToLower(result.Content)
	for _, phrase := range []string{"ad hoc", "carpe diem", "et cetera"} {
		if !strings.Contains(lower, phrase) {
			t.Errorf("expected extracted markdown to contain %q", phrase)
		}
	}
}
