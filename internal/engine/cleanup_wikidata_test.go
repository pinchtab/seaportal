package engine

import (
	"strings"
	"testing"
)

// regression: wikidata-edit-property-pencil-leak
func TestCleanupMarkdown_StripsWikidataEditProperty(t *testing.T) {
	in := `Country: Egypt [![تعديل قيمة خاصية (P17) في ويكي بيانات](https://commons.wikimedia.org/edit.svg)](https://www.wikidata.org/wiki/Q79#P17) and more text.`
	got := CleanupMarkdown(in)
	if strings.Contains(got, "تعديل") {
		t.Errorf("expected Wikidata edit-pencil to be stripped, got: %q", got)
	}
	if strings.Contains(got, "wikidata.org/wiki/Q79#P17") {
		t.Errorf("expected the wikidata link to be stripped, got: %q", got)
	}
	if !strings.Contains(got, "Country: Egypt") || !strings.Contains(got, "and more text") {
		t.Errorf("surrounding text lost, got: %q", got)
	}
}

// regression: wikidata-edit-property-pencil-leak
func TestCleanupMarkdown_PreservesNonWikidataImageLinks(t *testing.T) {
	in := `Gallery: [![photo caption](photo.jpg)](https://example.com/gallery/1) end.`
	got := CleanupMarkdown(in)
	if !strings.Contains(got, "photo.jpg") || !strings.Contains(got, "example.com/gallery/1") {
		t.Errorf("non-Wikidata image link was wrongly stripped, got: %q", got)
	}
}

// regression: wikidata-edit-property-pencil-leak
func TestCleanupMarkdown_PreservesWikidataItemLinks(t *testing.T) {
	// Bare Wikidata Q-page link without a #P property anchor — that's an item
	// reference, not a property-edit pencil. Keep it.
	in := `See [Q123 on Wikidata](https://www.wikidata.org/wiki/Q123) for details.`
	got := CleanupMarkdown(in)
	if !strings.Contains(got, "wikidata.org/wiki/Q123") {
		t.Errorf("bare Wikidata item link was wrongly stripped, got: %q", got)
	}
}
