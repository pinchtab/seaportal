package engine

type PageClass string

const (
	PageStatic   PageClass = "static"   // Plain HTML, no JS needed
	PageSSR      PageClass = "ssr"      // Server-side rendered (React/Next SSR, etc.)
	PageHydrated PageClass = "hydrated" // SSR with client-side hydration
	PageSPA      PageClass = "spa"      // Single-page app, JS required for content
	PageDynamic  PageClass = "dynamic"  // Personalized or frequently changing
	PageBlocked  PageClass = "blocked"  // Bot detection, captcha, rate limited
)

type ExtractionOutcome string

const (
	OutcomeExtract        ExtractionOutcome = "extract"              // Safe to use extraction result
	OutcomeExtractWarning ExtractionOutcome = "extract-with-warning" // Usable but may have issues
	OutcomeFailFast       ExtractionOutcome = "fail-fast"            // Low value, skip
	OutcomeNeedsBrowser   ExtractionOutcome = "needs-browser"        // Requires real browser
)

type PageProfile struct {
	Class       PageClass         `json:"class"`
	Outcome     ExtractionOutcome `json:"outcome"`
	Reasons     []string          `json:"reasons"`
	Confidence  int               `json:"confidence"`
	Trustworthy bool              `json:"trustworthy"`
}

func ClassifyPage(result Result) PageProfile {
	profile := PageProfile{
		Confidence: result.Confidence,
	}

	if result.IsBlocked {
		profile.Class = PageBlocked
		profile.Outcome = OutcomeNeedsBrowser
		profile.Reasons = append(profile.Reasons, "bot-detection-triggered")
		profile.Trustworthy = false
		return profile
	}

	if result.IsSPA {
		profile.Class = PageSPA
		profile.Outcome = OutcomeNeedsBrowser
		for _, sig := range result.SPASignals {
			profile.Reasons = append(profile.Reasons, "spa-signal:"+sig)
		}
		if result.Confidence < 30 {
			profile.Reasons = append(profile.Reasons, "low-confidence-extraction")
		}
		profile.Trustworthy = false
		return profile
	}

	if result.Confidence >= 80 {
		if hasHydrationMarkers(result) { //nolint:gocritic // complex conditions
			profile.Class = PageHydrated
			profile.Reasons = append(profile.Reasons, "high-confidence", "hydration-markers-present")
		} else if hasSSRMarkers(result) {
			profile.Class = PageSSR
			profile.Reasons = append(profile.Reasons, "high-confidence", "ssr-markers-present")
		} else {
			profile.Class = PageStatic
			profile.Reasons = append(profile.Reasons, "high-confidence", "plain-html")
		}
		profile.Outcome = OutcomeExtract
		profile.Trustworthy = true
		return profile
	}

	// Minimal static pages: low confidence due to short content, but no SPA signals
	// and high quality extraction (>80%) indicates a simple static page
	if isMinimalStaticPage(result) {
		profile.Class = PageStatic
		profile.Outcome = OutcomeExtract
		profile.Reasons = append(profile.Reasons, "minimal-content", "clean-extraction", "no-spa-signals")
		profile.Trustworthy = true
		return profile
	}

	// Medium confidence - could be hydrated or have some dynamic elements
	if result.Confidence >= 50 {
		if hasHydrationMarkers(result) { //nolint:gocritic // complex conditions
			profile.Class = PageHydrated
			profile.Reasons = append(profile.Reasons, "medium-confidence", "hydration-markers")
			profile.Outcome = OutcomeExtract
		} else {
			profile.Class = PageDynamic
			profile.Reasons = append(profile.Reasons, "medium-confidence", "possible-personalization")
			profile.Outcome = OutcomeExtractWarning
		}
		profile.Trustworthy = result.Confidence >= 60
		return profile
	}

	// Low confidence but not detected as SPA
	profile.Class = PageDynamic
	profile.Outcome = OutcomeExtractWarning
	profile.Reasons = append(profile.Reasons, "low-confidence", "content-may-be-incomplete")
	profile.Trustworthy = false

	if result.Confidence < 30 {
		profile.Outcome = OutcomeFailFast
		profile.Reasons = append(profile.Reasons, "very-low-extraction-quality")
	}

	return profile
}

// isMinimalStaticPage: specifically for example.com-style pages where low
// confidence is ONLY due to "minimal-body-content" signal (short static page)
func isMinimalStaticPage(result Result) bool {
	if result.Confidence < 40 || result.Confidence >= 80 || result.Length < 100 {
		return false
	}
	// Must have exactly "minimal-body-content" as only SPA signal
	// This is the key: confidence was lowered by short content, not actual SPA markers
	if len(result.SPASignals) != 1 || result.SPASignals[0] != "minimal-body-content" {
		return false
	}
	return true
}

// hasHydrationMarkers checks for signs of SSR + hydration
// Key distinction from SSR: hydrated pages have framework markers (React/Vue/etc.)
// but still have good server-rendered content
func hasHydrationMarkers(result Result) bool {
	hasSPARoot := false
	for _, sig := range result.SPASignals {
		if sig == "spa-root-element" {
			hasSPARoot = true
			break
		}
	}

	// Must have framework root element AND good content extraction
	if hasSPARoot && result.Confidence >= 50 && result.Length > 500 {
		return true
	}

	// Additional hydration indicator: high confidence + moderate structure
	// but content length suggests JS-enhanced rendering
	if hasSPARoot && result.HeadingCount >= 1 && result.ParagraphCount >= 2 {
		return true
	}

	return false
}

// hasSSRMarkers: good structure (headings + paragraphs) suggests CMS or framework
func hasSSRMarkers(result Result) bool {
	return result.HeadingCount >= 2 && result.ParagraphCount >= 3 && result.Length > 1000
}

func (p PageProfile) String() string {
	status := "✓"
	if !p.Trustworthy {
		status = "⚠"
	}
	return string(p.Class) + " " + status + " → " + string(p.Outcome)
}
