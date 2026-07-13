package engine

import (
	"strings"
)

type PageClass string

const (
	PageStatic   PageClass = "static"
	PageSSR      PageClass = "ssr"
	PageHydrated PageClass = "hydrated"
	PageSPA      PageClass = "spa"
	PageDynamic  PageClass = "dynamic"
	PageBlocked  PageClass = "blocked"
)

type ExtractionOutcome string

const (
	OutcomeExtract        ExtractionOutcome = "extract"
	OutcomeExtractWarning ExtractionOutcome = "extract-with-warning"
	OutcomeFailFast       ExtractionOutcome = "fail-fast"
	OutcomeNeedsBrowser   ExtractionOutcome = "needs-browser"
)

type BrowserDecision string

const (
	DecisionStaticHighConfidence BrowserDecision = "static-high-confidence"
	DecisionStaticOK             BrowserDecision = "static-ok"
	DecisionStaticCaution        BrowserDecision = "static-caution"
	DecisionBrowserNeeded        BrowserDecision = "browser-needed"
	DecisionBlocked              BrowserDecision = "blocked"
	DecisionUnreachable          BrowserDecision = "unreachable"
	DecisionNotFound             BrowserDecision = "not-found"
	DecisionUnsupported          BrowserDecision = "unsupported"
)

const (
	confHighGate        = 80
	confMediumGate      = 50
	confTrustworthyGate = 60
	confVeryLowGate     = 30
)

const (
	hydratedMinLength     = 2000
	hydratedMinHeadings   = 2
	hydratedMinParagraphs = 3
)

const (
	fallbackStaticMinLength  = 500
	staticBulkMinLength      = 2000
	staticShortMinConfidence = 70
	staticShortMinLength     = 800
	staticThinMinLength      = 300
)

type PageProfile struct {
	Class              PageClass         `json:"class"`
	Outcome            ExtractionOutcome `json:"outcome"`
	Decision           BrowserDecision   `json:"decision"`
	BrowserRecommended bool              `json:"browserRecommended"`
	Reasons            []string          `json:"reasons"`
	Confidence         int               `json:"confidence"`
	Trustworthy        bool              `json:"trustworthy"`
}

func ClassifyPage(result Result) PageProfile {
	profile := classifyPageInternal(result)
	profile.Decision, profile.BrowserRecommended = deriveDecision(result, profile)
	return profile
}

func classifyPageInternal(result Result) PageProfile {
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
		if result.Length > hydratedMinLength && result.HeadingCount >= hydratedMinHeadings && result.ParagraphCount >= hydratedMinParagraphs {
			profile.Class = PageHydrated
			profile.Outcome = OutcomeExtract
			profile.Reasons = append(profile.Reasons, "spa-bootstrap-with-real-content")
			for _, sig := range result.SPASignals {
				profile.Reasons = append(profile.Reasons, "spa-signal:"+sig)
			}
			profile.Trustworthy = result.Confidence >= confMediumGate
			return profile
		}
		profile.Class = PageSPA
		profile.Outcome = OutcomeNeedsBrowser
		for _, sig := range result.SPASignals {
			profile.Reasons = append(profile.Reasons, "spa-signal:"+sig)
		}
		if result.Confidence < confVeryLowGate {
			profile.Reasons = append(profile.Reasons, "low-confidence-extraction")
		}
		profile.Trustworthy = false
		return profile
	}

	if isJSShellContent(result) {
		profile.Class = PageSPA
		profile.Outcome = OutcomeNeedsBrowser
		profile.Reasons = append(profile.Reasons, "js-shell-content")
		profile.Trustworthy = false
		return profile
	}

	if result.Confidence >= confHighGate {
		if hasHydrationMarkers(result) { //nolint:gocritic
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
		if triggered, reason := detectAuthWallByContent(result); triggered {
			profile.Outcome = OutcomeNeedsBrowser
			profile.Reasons = append(profile.Reasons, reason)
			profile.Trustworthy = false
		}
		return profile
	}

	if isMinimalStaticPage(result) {
		profile.Class = PageStatic
		profile.Outcome = OutcomeExtract
		profile.Reasons = append(profile.Reasons, "minimal-content", "clean-extraction", "no-spa-signals")
		profile.Trustworthy = true
		return profile
	}

	if result.Confidence >= confMediumGate {
		return classifyMediumConfidence(result)
	}

	profile.Class = PageDynamic
	profile.Outcome = OutcomeExtractWarning
	profile.Reasons = append(profile.Reasons, "low-confidence", "content-may-be-incomplete")
	profile.Trustworthy = false

	if result.Confidence < confVeryLowGate {
		profile.Outcome = OutcomeFailFast
		profile.Reasons = append(profile.Reasons, "very-low-extraction-quality")
	}

	return profile
}

func classifyMediumConfidence(result Result) PageProfile {
	profile := PageProfile{
		Confidence: result.Confidence,
	}

	switch {
	case hasHydrationMarkers(result):
		profile.Class = PageHydrated
		profile.Reasons = append(profile.Reasons, "medium-confidence", "hydration-markers")
		profile.Outcome = OutcomeExtract
	case hasMediumSSRStructure(result):
		profile.Class = PageSSR
		profile.Reasons = append(profile.Reasons, "medium-confidence", "ssr-structure")
		profile.Outcome = OutcomeExtract
	case hasMediumStaticIndex(result):
		profile.Class = PageStatic
		profile.Reasons = append(profile.Reasons, "medium-confidence", "static-index")
		profile.Outcome = OutcomeExtract
	case hasMediumStaticShape(result):
		profile.Class = PageStatic
		profile.Reasons = append(profile.Reasons, "medium-confidence", "plain-html", "no-spa-signals")
		profile.Outcome = OutcomeExtract
	default:
		hasContent := result.Length >= fallbackStaticMinLength || result.HeadingCount >= 1 || result.ParagraphCount >= 2
		isErrorResponse := result.StatusCode >= 400
		if len(result.SPASignals) > 0 || result.Confidence < confMediumGate || !hasContent || isErrorResponse {
			profile.Class = PageDynamic
			profile.Reasons = append(profile.Reasons, "medium-confidence", "possible-personalization")
			profile.Outcome = OutcomeExtractWarning
		} else {
			profile.Class = PageStatic
			profile.Reasons = append(profile.Reasons, "medium-confidence", "fallback-static", "no-positive-signals")
			profile.Outcome = OutcomeExtract
		}
	}

	profile.Trustworthy = result.Confidence >= confTrustworthyGate
	if profile.Outcome == OutcomeExtract {
		if triggered, reason := detectAuthWallByContent(result); triggered {
			profile.Outcome = OutcomeNeedsBrowser
			profile.Reasons = append(profile.Reasons, reason)
			profile.Trustworthy = false
		}
	}
	return profile
}

func deriveDecision(result Result, profile PageProfile) (BrowserDecision, bool) {
	if isBinarySkipError(result.Error) || isBinaryContentType(result.ResponseContentType) {
		return DecisionUnsupported, false
	}
	if result.Error != "" && result.StatusCode == 0 {
		return DecisionUnreachable, false
	}
	if result.StatusCode == 404 || result.IsSoft404 || reasonsContain(profile.Reasons, "http-404-not-found") {
		return DecisionNotFound, false
	}
	if result.IsBlocked || profile.Class == PageBlocked {
		return DecisionBlocked, true
	}

	okStatus := result.StatusCode == 0 || (result.StatusCode >= 200 && result.StatusCode < 300)

	switch profile.Outcome {
	case OutcomeNeedsBrowser, OutcomeFailFast:
		return DecisionBrowserNeeded, true
	case OutcomeExtract:
		staticClass := profile.Class == PageStatic || profile.Class == PageSSR || profile.Class == PageHydrated
		if okStatus && staticClass && profile.Trustworthy && result.Length >= 1000 {
			return DecisionStaticHighConfidence, false
		}
		if okStatus && result.Length >= 500 {
			return DecisionStaticOK, false
		}
		return DecisionStaticCaution, false
	case OutcomeExtractWarning:
		if result.Length >= 500 {
			return DecisionStaticCaution, false
		}
		return DecisionBrowserNeeded, true
	default:
		return DecisionBrowserNeeded, true
	}
}

func isBinarySkipError(errStr string) bool {
	return strings.HasPrefix(errStr, "skipped binary content")
}

func reasonsContain(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

var jsShellPhrases = []string{
	"enable javascript",
	"javascript is enabled",
	"javascript is required",
	"javascript required",
	"requires javascript",
	"javascript is disabled",
	"javascript is not enabled",
	"javascript must be enabled",
	"turn on javascript",
}

func isJSShellContent(result Result) bool {
	if result.Length <= 0 || result.Length >= 500 {
		return false
	}
	content := strings.ToLower(result.Content)
	for _, phrase := range jsShellPhrases {
		if strings.Contains(content, phrase) {
			return true
		}
	}
	if result.ParagraphCount == 0 &&
		(strings.Contains(content, "loading...") || strings.Contains(content, "loading…")) {
		return true
	}
	return false
}

func isMinimalStaticPage(result Result) bool {
	if result.Confidence < 40 || result.Confidence >= 80 || result.Length < 100 {
		return false
	}
	if len(result.SPASignals) != 1 || result.SPASignals[0] != "minimal-body-content" {
		return false
	}
	return true
}

func hasHydrationMarkers(result Result) bool {
	hasSPARoot := false
	for _, sig := range result.SPASignals {
		if sig == "spa-root-element" {
			hasSPARoot = true
			break
		}
	}

	if hasSPARoot && result.Confidence >= 50 && result.Length > 500 {
		return true
	}

	if hasSPARoot && result.HeadingCount >= 1 && result.ParagraphCount >= 2 {
		return true
	}

	return false
}

func hasSSRMarkers(result Result) bool {
	return result.HeadingCount >= 2 && result.ParagraphCount >= 3 && result.Length > 1000
}

var benignSPASignals = map[string]bool{
	"ldjson-supplemented": true,
	"index-page-fallback": true,
}

func hasOnlyBenignSPASignals(result Result) bool {
	for _, sig := range result.SPASignals {
		if !benignSPASignals[sig] {
			return false
		}
	}
	return true
}

func hasMediumSSRStructure(result Result) bool {
	if result.IsSPA {
		return false
	}
	if !hasOnlyBenignSPASignals(result) {
		return false
	}
	if result.HeadingCount < 2 || result.ParagraphCount < 3 {
		return false
	}
	return result.Length >= 1000
}

func hasMediumStaticIndex(result Result) bool {
	if result.IsSPA {
		return false
	}
	if !hasOnlyBenignSPASignals(result) {
		return false
	}
	return result.HeadingCount >= 5 && result.Length >= 300
}

func hasMediumStaticShape(result Result) bool {
	if result.IsSPA {
		return false
	}
	if !hasOnlyBenignSPASignals(result) {
		return false
	}
	if result.Length >= staticBulkMinLength {
		return true
	}
	if result.Confidence >= staticShortMinConfidence {
		if result.Length >= staticShortMinLength {
			return true
		}
		if result.Length >= staticThinMinLength && result.ParagraphCount >= 1 {
			return true
		}
	}
	return false
}

func ensureProfile(result *Result) {
	if result == nil {
		return
	}
	if result.Profile.Class == "" {
		result.Profile = ClassifyPage(*result)
	}
	result.PageClass = result.Profile.Class
	result.Profile.Decision, result.Profile.BrowserRecommended = deriveDecision(*result, result.Profile)
}

func hasLoginFormMarkers(loweredContent string) bool {
	if strings.Contains(loweredContent, `type="password"`) {
		return true
	}
	hasEmailField := strings.Contains(loweredContent, "email or phone") ||
		strings.Contains(loweredContent, "email address") ||
		strings.Contains(loweredContent, "phone or email")
	hasPasswordField := strings.Contains(loweredContent, "password")
	if hasEmailField && hasPasswordField {
		return true
	}
	return false
}

func (p PageProfile) String() string {
	status := "✓"
	if !p.Trustworthy {
		status = "⚠"
	}
	return string(p.Class) + " " + status + " → " + string(p.Outcome)
}
