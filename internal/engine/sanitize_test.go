package engine

import (
	"strings"
	"testing"
)

func TestSanitizeHTML_NestedAriaHidden(t *testing.T) {
	input := `<div aria-hidden="true"><div>inner</div></div><p>keep</p>`
	got := SanitizeHTML(input)

	if strings.Contains(got, "inner") {
		t.Errorf("nested aria-hidden block was not fully removed: %q", got)
	}
	if strings.Contains(got, "</div>") {
		t.Errorf("outer </div> leaked through (broken HTML): %q", got)
	}
	if !strings.Contains(got, "<p>keep</p>") {
		t.Errorf("sibling content was incorrectly removed: %q", got)
	}
}

func TestSanitizeHTML_NestedHiddenAttr(t *testing.T) {
	input := `<section hidden><section>nested</section></section><p>visible</p>`
	got := SanitizeHTML(input)

	if strings.Contains(got, "nested") {
		t.Errorf("nested hidden block was not fully removed: %q", got)
	}
	if strings.Contains(got, "</section>") {
		t.Errorf("outer </section> leaked through: %q", got)
	}
	if !strings.Contains(got, "<p>visible</p>") {
		t.Errorf("sibling content was incorrectly removed: %q", got)
	}
}

func TestSanitizeHTML_SiblingsBothHidden(t *testing.T) {
	input := `<div aria-hidden="true">a</div><div aria-hidden="true">b</div><p>c</p>`
	got := SanitizeHTML(input)

	if strings.Contains(got, ">a<") || strings.Contains(got, ">b<") {
		t.Errorf("hidden siblings were not removed: %q", got)
	}
	if !strings.Contains(got, "<p>c</p>") {
		t.Errorf("trailing sibling lost: %q", got)
	}
}

func TestSanitizeHTML_DeepNesting(t *testing.T) {
	input := `<div aria-hidden="true"><div><div>deep</div></div></div>after`
	got := SanitizeHTML(input)

	if strings.Contains(got, "deep") {
		t.Errorf("deeply nested content not removed: %q", got)
	}
	if strings.Contains(got, "</div>") {
		t.Errorf("close tag leaked: %q", got)
	}
	if !strings.Contains(got, "after") {
		t.Errorf("trailing text lost: %q", got)
	}
}
