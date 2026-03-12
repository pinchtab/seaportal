package portal

import "testing"

func TestClassifyPage_Static(t *testing.T) {
	// Static = high confidence, no SSR markers (few headings/paragraphs)
	result := Result{
		Confidence:     85,
		HeadingCount:   1,
		ParagraphCount: 2,
		Length:         800,
		IsSPA:          false,
		IsBlocked:      false,
	}

	profile := ClassifyPage(result)

	if profile.Class != PageStatic {
		t.Errorf("expected static, got %s", profile.Class)
	}
	if profile.Outcome != OutcomeExtract {
		t.Errorf("expected extract, got %s", profile.Outcome)
	}
	if !profile.Trustworthy {
		t.Error("expected trustworthy for high-confidence static")
	}
}

func TestClassifyPage_SPA(t *testing.T) {
	result := Result{
		Confidence: 20,
		Length:     100,
		IsSPA:      true,
		SPASignals: []string{"spa-root-element", "minimal-body-content"},
	}

	profile := ClassifyPage(result)

	if profile.Class != PageSPA {
		t.Errorf("expected spa, got %s", profile.Class)
	}
	if profile.Outcome != OutcomeNeedsBrowser {
		t.Errorf("expected needs-browser, got %s", profile.Outcome)
	}
	if profile.Trustworthy {
		t.Error("SPA should not be trustworthy")
	}
}

func TestClassifyPage_Blocked(t *testing.T) {
	result := Result{
		Confidence: 15,
		IsBlocked:  true,
		IsSPA:      false,
	}

	profile := ClassifyPage(result)

	if profile.Class != PageBlocked {
		t.Errorf("expected blocked, got %s", profile.Class)
	}
	if profile.Outcome != OutcomeNeedsBrowser {
		t.Errorf("expected needs-browser, got %s", profile.Outcome)
	}
}

func TestClassifyPage_SSR(t *testing.T) {
	// SSR = high confidence with good structure (multiple headings/paragraphs)
	result := Result{
		Confidence:     85,
		HeadingCount:   5,
		ParagraphCount: 10,
		Length:         2000,
		IsSPA:          false,
		IsBlocked:      false,
	}

	profile := ClassifyPage(result)

	if profile.Class != PageSSR {
		t.Errorf("expected ssr, got %s", profile.Class)
	}
	if profile.Outcome != OutcomeExtract {
		t.Errorf("expected extract, got %s", profile.Outcome)
	}
	if !profile.Trustworthy {
		t.Error("expected trustworthy for high-confidence SSR")
	}
}

func TestClassifyPage_Hydrated(t *testing.T) {
	result := Result{
		Confidence:     70,
		HeadingCount:   3,
		ParagraphCount: 5,
		Length:         1500,
		IsSPA:          false,
		SPASignals:     []string{"spa-root-element"}, // Has root but good content
	}

	profile := ClassifyPage(result)

	if profile.Class != PageHydrated {
		t.Errorf("expected hydrated, got %s", profile.Class)
	}
	if profile.Outcome != OutcomeExtract {
		t.Errorf("expected extract, got %s", profile.Outcome)
	}
}

func TestClassifyPage_Dynamic(t *testing.T) {
	result := Result{
		Confidence:     55,
		HeadingCount:   1,
		ParagraphCount: 2,
		Length:         500,
		IsSPA:          false,
		SPASignals:     nil,
	}

	profile := ClassifyPage(result)

	if profile.Class != PageDynamic {
		t.Errorf("expected dynamic, got %s", profile.Class)
	}
	if profile.Outcome != OutcomeExtractWarning {
		t.Errorf("expected extract-with-warning, got %s", profile.Outcome)
	}
}

func TestClassifyPage_LowConfidenceFailFast(t *testing.T) {
	result := Result{
		Confidence:     25,
		HeadingCount:   0,
		ParagraphCount: 0,
		Length:         50,
		IsSPA:          false, // Not detected as SPA but very low quality
	}

	profile := ClassifyPage(result)

	if profile.Class != PageDynamic {
		t.Errorf("expected dynamic, got %s", profile.Class)
	}
	if profile.Outcome != OutcomeFailFast {
		t.Errorf("expected fail-fast for very low confidence, got %s", profile.Outcome)
	}
}

func TestPageProfile_String(t *testing.T) {
	profile := PageProfile{
		Class:       PageStatic,
		Outcome:     OutcomeExtract,
		Trustworthy: true,
	}

	s := profile.String()
	if s != "static ✓ → extract" {
		t.Errorf("unexpected string: %s", s)
	}

	profile.Trustworthy = false
	s = profile.String()
	if s != "static ⚠ → extract" {
		t.Errorf("unexpected string for untrustworthy: %s", s)
	}
}
