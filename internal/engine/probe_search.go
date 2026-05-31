package engine

import (
	"net/url"
	"regexp"
	"strings"
)

const probeSearchMinLength = 600

// Sentinel string emitted on Profile.Reasons and Validation.Issues; downstream
// agents grep for this exact value, do not change without a coordinated update.
const probeSearchReason = "client-rendered-search"

var (
	reSearchLinkListItem       = regexp.MustCompile(`(?m)^-\s+\[[^\]]+\]\([^)]+\)`)
	reSearchLinkHeading        = regexp.MustCompile(`(?m)^#{2,6}\s+\[[^\]]+\]\([^)]+\)`)
	reSearchHeadingWithResults = regexp.MustCompile(`(?im)^#{1,6}\s+.*\b(results|search)\b`)
	reNumericResults           = regexp.MustCompile(`(?i)\b[\d,]+\s+(results|risultati|hits)\b`)
	reSearchQueryParam         = regexp.MustCompile(`(?i)(^|[?&])(q|query|search)=`)
)

func looksLikeSearchURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	path := strings.ToLower(u.Path)
	if strings.Contains(path, "/search") || strings.Contains(path, "/srp") {
		return true
	}
	if u.RawQuery != "" && reSearchQueryParam.MatchString(u.RawQuery) {
		return true
	}
	return false
}

func looksLikeSearchResults(markdown string) bool {
	if markdown == "" {
		return false
	}
	if matches := reSearchLinkListItem.FindAllStringIndex(markdown, -1); len(matches) >= 3 {
		return true
	}
	if matches := reSearchLinkHeading.FindAllStringIndex(markdown, -1); len(matches) >= 3 {
		return true
	}
	if reSearchHeadingWithResults.MatchString(markdown) {
		return true
	}
	if reNumericResults.MatchString(markdown) {
		return true
	}
	return false
}

func resultsMentionQuery(rawURL, markdown string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	q := u.Query()
	term := ""
	for _, k := range []string{"q", "query", "search"} {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			term = v
			break
		}
	}
	if len(term) < 3 {
		return true
	}
	return strings.Contains(strings.ToLower(markdown), strings.ToLower(term))
}

func applyProbeSearchOverride(result *Result, opts Options) {
	if !opts.ProbeSearch || result == nil {
		return
	}
	switch result.Profile.Class {
	case PageStatic, PageSSR, PageHydrated:
	default:
		return
	}
	if !looksLikeSearchURL(result.URL) {
		return
	}
	hasStructure := looksLikeSearchResults(result.Content)
	mentionsQuery := resultsMentionQuery(result.URL, result.Content)
	if result.Length >= probeSearchMinLength && hasStructure && mentionsQuery {
		return
	}
	result.Profile.Outcome = OutcomeNeedsBrowser
	result.Profile.Reasons = append(result.Profile.Reasons, probeSearchReason)
	result.Validation.NeedsBrowser = true
	if !containsIssue(result.Validation.Issues, probeSearchReason) {
		result.Validation.Issues = append(result.Validation.Issues, probeSearchReason)
	}
}

func containsIssue(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
