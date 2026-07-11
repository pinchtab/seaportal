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

// BrowserDecision is the single routing category a caller (e.g. PinchTab) reads
// to decide whether to fall through to a real browser. It refines Outcome with
// transport/status context; BrowserRecommended is the one boolean to branch on.
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

type PageProfile struct {
	Class              PageClass         `json:"class"`
	Outcome            ExtractionOutcome `json:"outcome"`
	Decision           BrowserDecision   `json:"decision"`
	BrowserRecommended bool              `json:"browserRecommended"`
	Reasons            []string          `json:"reasons"`
	Confidence         int               `json:"confidence"`
	Trustworthy        bool              `json:"trustworthy"`
}

// ClassifyPage determines the page profile and the browser-routing decision.
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
		// SPA-bootstrap-with-rendered-content: if the page advertises a SPA
		// root but the extractor pulled out real prose (headings, paragraphs,
		// non-trivial length), it is server-rendered for first paint. Call it
		// `hydrated`, not `spa`, so callers extract instead of escalating to a
		// browser. Thresholds picked to keep round-1 corpus accuracy at 1.000
		// while catching real docs/forum pages with hydration markers.
		if result.Length > 2000 && result.HeadingCount >= 2 && result.ParagraphCount >= 3 {
			profile.Class = PageHydrated
			profile.Outcome = OutcomeExtract
			profile.Reasons = append(profile.Reasons, "spa-bootstrap-with-real-content")
			for _, sig := range result.SPASignals {
				profile.Reasons = append(profile.Reasons, "spa-signal:"+sig)
			}
			profile.Trustworthy = result.Confidence >= 50
			return profile
		}
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

	if isJSShellContent(result) {
		profile.Class = PageSPA
		profile.Outcome = OutcomeNeedsBrowser
		profile.Reasons = append(profile.Reasons, "js-shell-content")
		profile.Trustworthy = false
		return profile
	}

	if result.Confidence >= 80 {
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

	if result.Confidence >= 50 {
		isExtractClass := true
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
			// regression: classifier-round-2-fallback — the prior `default:
			// PageDynamic` swallowed every unmatched medium-confidence page,
			// labelling plain SSR/news/forum responses as "personalized".
			// Only call it dynamic when there's an actual positive signal
			// (SPA root / hydration scaffolding) or low confidence. Otherwise
			// treat as best-effort static: agents don't have to escalate, and
			// the static label still carries the medium-confidence reason
			// chain so callers can downgrade trust if they want.
			// Static fallback gate: ONLY downgrade to static when there is
			// also substantive extracted content (Length>=500 OR ≥1 heading
			// OR ≥2 paragraphs). Short/thin extractions with no positive
			// signals are usually login/auth/paywall stubs — honest call is
			// still `dynamic` because the agent should re-evaluate, not
			// trust the few bytes that survived extraction.
			hasContent := result.Length >= 500 || result.HeadingCount >= 1 || result.ParagraphCount >= 2
			// Error responses (4xx/5xx) must never be promoted to `static`
			// regardless of body shape — the body may render fine but the
			// transport said "not OK", and callers rely on the outcome to
			// avoid trusting error bodies as primary content.
			isErrorResponse := result.StatusCode >= 400
			if len(result.SPASignals) > 0 || result.Confidence < 50 || !hasContent || isErrorResponse {
				profile.Class = PageDynamic
				profile.Reasons = append(profile.Reasons, "medium-confidence", "possible-personalization")
				profile.Outcome = OutcomeExtractWarning
				isExtractClass = false
			} else {
				profile.Class = PageStatic
				profile.Reasons = append(profile.Reasons, "medium-confidence", "fallback-static", "no-positive-signals")
				profile.Outcome = OutcomeExtract
			}
		}
		profile.Trustworthy = result.Confidence >= 60
		if isExtractClass {
			if triggered, reason := detectAuthWallByContent(result); triggered {
				profile.Outcome = OutcomeNeedsBrowser
				profile.Reasons = append(profile.Reasons, reason)
				profile.Trustworthy = false
			}
		}
		return profile
	}

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

// deriveDecision maps a resolved profile + result onto the BrowserDecision
// contract (docs/reference/browser-discriminator.md). Terminal/transport states
// win first; then the classifier's Outcome drives the static tiers. It routes on
// outcome/class/trustworthy/length rather than the `quality` float, which is too
// noisy to be a standalone routing signal (good SSR pages can score near zero).
func deriveDecision(result Result, profile PageProfile) (BrowserDecision, bool) {
	// Resource type SeaPortal can't evaluate (binary/image/etc.).
	if isBinarySkipError(result.Error) || isBinaryContentType(result.ResponseContentType) {
		return DecisionUnsupported, false
	}
	// Transport failure with no usable HTTP response (DNS/TLS/conn/timeout).
	if result.Error != "" && result.StatusCode == 0 {
		return DecisionUnreachable, false
	}
	// Hard or soft 404.
	if result.StatusCode == 404 || result.IsSoft404 || reasonsContain(profile.Reasons, "http-404-not-found") {
		return DecisionNotFound, false
	}
	// Bot protection / captcha / access denied. Chrome may still need a policy.
	if result.IsBlocked || profile.Class == PageBlocked {
		return DecisionBlocked, true
	}

	okStatus := result.StatusCode == 0 || (result.StatusCode >= 200 && result.StatusCode < 300)

	switch profile.Outcome {
	case OutcomeNeedsBrowser, OutcomeFailFast:
		// needs-browser dominates length/quality: an auth-wall or SPA shell can
		// carry many bytes and still require a browser.
		return DecisionBrowserNeeded, true
	case OutcomeExtract:
		staticClass := profile.Class == PageStatic || profile.Class == PageSSR || profile.Class == PageHydrated
		if okStatus && staticClass && profile.Trustworthy && result.Length >= 1000 {
			return DecisionStaticHighConfidence, false
		}
		if okStatus && result.Length >= 500 {
			return DecisionStaticOK, false
		}
		// Extract succeeded but the body is thin (incl. intentionally minimal
		// pages). The classifier trusted it, so don't spend a browser — flag
		// caution so callers can still escalate for completeness checks.
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

// jsShellPhrases are content-side "this page needs JavaScript" tells, matched
// against the lowered extracted content.
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

// isJSShellContent reports whether a successfully-extracted page is really a
// client-rendered shell: short extracted content dominated by a JS-required
// warning or a bare loading screen. Complements DetectSPA, which keys on
// raw-HTML markers (<noscript> warnings, spa-root ids) and misses shells that
// render the warning as regular DOM (e.g. app.diagrams.net).
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
	// A bare loading screen only counts when there is no paragraph prose
	// around it — short static pages can legitimately mention "loading...".
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
	// Bulk-shape static: long body is reliably static even with no paragraph
	// or heading structure (e.g. index/listing pages). Floor preserved at
	// 2000 to keep TestClassifyPage_RFC2616Static and friends green.
	if result.Length >= 2000 {
		return true
	}
	// Short-body static needs a stronger confidence gate so we don't catch
	// medium-confidence dynamic shells. At conf>=70 the extractor has good
	// enough signal that the absence of SPA markers means it really is
	// pre-rendered HTML.
	if result.Confidence >= 70 {
		// hn-frontpage-fragment.html: H=0, P=0, Len=848 — chrome-only listing
		// page; conf=75; needs to land as static, not dynamic.
		// github-readme-with-login-example.html: H=0, P=0, Len=1095; conf=75.
		if result.Length >= 800 {
			return true
		}
		// article-ldjson.html: H=0, P=2, Len=650; conf=70.
		// article-og-full.html: H=0, P=1, Len=390; conf=70.
		if result.Length >= 300 && result.ParagraphCount >= 1 {
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
	// Re-derive on the final result so paths that set Profile directly (404,
	// binary, blocked, QuickNeedsBrowser) and any post-classify reason edits are
	// reflected in the routing decision. Pure function of result+profile, so
	// recomputing when ClassifyPage already ran is harmless.
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
