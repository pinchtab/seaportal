package engine

import (
	"strings"
	"testing"
)

func makeFiller(n int) string {
	const line = "This is a long-form encyclopedic paragraph with substantive prose content that exists purely to inflate the extracted body length past the bodyTrusted threshold used by the validator. "
	var b strings.Builder
	for b.Len() < n {
		b.WriteString(line)
		b.WriteString("\n\n")
	}
	return b.String()
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestValidate_LargeWikipediaLikePage(t *testing.T) {
	body := makeFiller(30000) + "\n\nThe site is served behind cloudflare for performance.\n"
	r := &Result{
		Content:        body,
		Length:         99639,
		HeadingCount:   5,
		ParagraphCount: 97,
		LinkCount:      871,
	}

	v := ValidateExtraction(r)

	if !v.IsValid {
		t.Fatalf("expected IsValid=true for trusted large body, got false; issues=%v confidence=%v", v.Issues, v.Confidence)
	}
	if v.NeedsBrowser {
		t.Fatalf("expected NeedsBrowser=false for trusted large body, got true; issues=%v", v.Issues)
	}
	if v.LinkDensity <= 5.0 {
		t.Fatalf("expected LinkDensity > 5.0, got %v", v.LinkDensity)
	}
}

func TestValidate_StubBotProtectionPage(t *testing.T) {
	r := &Result{
		Content:        "Checking your browser, please wait while we verify you are human.",
		Length:         500,
		HeadingCount:   0,
		ParagraphCount: 1,
		LinkCount:      0,
	}

	v := ValidateExtraction(r)

	if v.IsValid {
		t.Fatalf("expected IsValid=false on stub bot-protection page, got true; issues=%v", v.Issues)
	}
	if !v.NeedsBrowser {
		t.Fatalf("expected NeedsBrowser=true on stub bot-protection page, got false")
	}
	if !contains(v.Issues, "bot-protection") {
		t.Fatalf("expected 'bot-protection' issue, got %v", v.Issues)
	}
}

func TestValidate_NavHeavyShortPage(t *testing.T) {
	body := makeFiller(2000)
	r := &Result{
		Content:        body,
		Length:         2000,
		HeadingCount:   1,
		ParagraphCount: 2,
		LinkCount:      50,
	}

	v := ValidateExtraction(r)

	if !contains(v.Issues, "nav-heavy") {
		t.Fatalf("expected 'nav-heavy' issue on thin high-density page, got %v", v.Issues)
	}
	if !v.NeedsBrowser {
		t.Fatalf("expected NeedsBrowser=true on thin high-density page (density>5), got false; issues=%v", v.Issues)
	}
}
