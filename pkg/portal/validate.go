// Package portal provides content extraction with SPA detection
package portal

import (
	"regexp"
	"strings"
)

// Validation holds post-extraction validation results
type Validation struct {
	IsValid       bool     `json:"isValid"`
	NeedsBrowser  bool     `json:"needsBrowser"`
	Confidence    float64  `json:"confidence"`    // 0-1 confidence in extraction quality
	Issues        []string `json:"issues"`        // List of detected issues
	LinkDensity   float64  `json:"linkDensity"`   // Links per paragraph (high = nav-heavy)
	SkeletonScore float64  `json:"skeletonScore"` // 0-1 how much content looks like skeleton/stubs
	ContentRatio  float64  `json:"contentRatio"`  // Ratio of real content vs boilerplate
}

var (
	// Skeleton patterns - content that looks like loading stubs
	skeletonPatterns = []string{
		"view more",
		"load more",
		"loading...",
		"please wait",
		"enable javascript",
		"javascript is required",
		"javascript must be enabled",
		"browser doesn't support",
		"upgrade your browser",
		"click here to",
		"sign in to",
		"log in to continue",
		"subscribe to",
		"create an account",
	}

	// Nav-heavy patterns - likely navigation not content

	// Bot protection patterns
	botProtectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)checking your browser`),
		regexp.MustCompile(`(?i)please wait while we verify`),
		regexp.MustCompile(`(?i)access denied`),
		regexp.MustCompile(`(?i)cloudflare`),
		regexp.MustCompile(`(?i)ddos protection`),
		regexp.MustCompile(`(?i)captcha`),
		regexp.MustCompile(`(?i)are you a robot`),
		regexp.MustCompile(`(?i)verify you are human`),
	}

	// Paywall patterns
	paywallPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)subscribe to read`),
		regexp.MustCompile(`(?i)subscribers only`),
		regexp.MustCompile(`(?i)premium content`),
		regexp.MustCompile(`(?i)unlock this article`),
		regexp.MustCompile(`(?i)free articles remaining`),
		regexp.MustCompile(`(?i)sign in to read`),
		regexp.MustCompile(`(?i)membership required`),
	}
)

func ValidateExtraction(r *Result) Validation {
	v := Validation{
		IsValid:    true,
		Confidence: 1.0,
	}

	content := strings.ToLower(r.Content)
	lines := strings.Split(r.Content, "\n")

	// Check for empty/minimal content
	wordCount := len(strings.Fields(r.Content))
	if wordCount < 50 {
		v.Issues = append(v.Issues, "minimal-content")
		v.Confidence -= 0.3
		if wordCount < 10 {
			v.NeedsBrowser = true
			v.IsValid = false
		}
	}

	// Calculate link density (links per paragraph)
	if r.ParagraphCount > 0 {
		v.LinkDensity = float64(r.LinkCount) / float64(r.ParagraphCount)
		if v.LinkDensity > 3.0 {
			v.Issues = append(v.Issues, "nav-heavy")
			v.Confidence -= 0.2
		}
		if v.LinkDensity > 5.0 {
			v.NeedsBrowser = true
		}
	}

	// Check for skeleton/stub content
	skeletonHits := 0
	for _, pattern := range skeletonPatterns {
		if strings.Contains(content, pattern) {
			skeletonHits++
		}
	}
	v.SkeletonScore = float64(skeletonHits) / float64(len(skeletonPatterns))
	if v.SkeletonScore > 0.2 {
		v.Issues = append(v.Issues, "skeleton-content")
		v.Confidence -= 0.3
		v.NeedsBrowser = true
	}

	// Check for bot protection
	for _, pattern := range botProtectionPatterns {
		if pattern.MatchString(content) {
			v.Issues = append(v.Issues, "bot-protection")
			v.NeedsBrowser = true
			v.IsValid = false
			v.Confidence = 0.1
			break
		}
	}

	// Check for paywall
	for _, pattern := range paywallPatterns {
		if pattern.MatchString(content) {
			v.Issues = append(v.Issues, "paywall-detected")
			v.Confidence -= 0.2
			break
		}
	}

	// Check heading-to-content ratio (many headings, little content = nav)
	if r.HeadingCount > 0 && r.ParagraphCount > 0 {
		headingRatio := float64(r.HeadingCount) / float64(r.ParagraphCount)
		if headingRatio > 0.8 {
			v.Issues = append(v.Issues, "heading-heavy")
			v.Confidence -= 0.15
		}
	}

	// Check for repetitive short lines (menu items)
	shortLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 0 && len(trimmed) < 30 {
			shortLines++
		}
	}
	if len(lines) > 0 {
		shortRatio := float64(shortLines) / float64(len(lines))
		if shortRatio > 0.7 {
			v.Issues = append(v.Issues, "menu-like")
			v.Confidence -= 0.2
		}
	}

	// Calculate content ratio (real paragraphs vs total elements)
	totalElements := r.HeadingCount + r.ParagraphCount + r.LinkCount
	if totalElements > 0 {
		v.ContentRatio = float64(r.ParagraphCount) / float64(totalElements)
	}

	// Clamp confidence
	if v.Confidence < 0 {
		v.Confidence = 0
	}
	if v.Confidence < 0.5 {
		v.IsValid = false
	}

	return v
}

// QuickNeedsBrowser performs fast pre-extraction check on raw HTML
// Returns true if browser is definitely needed (fast-mode bail)
func QuickNeedsBrowser(html string) (needsBrowser bool, reason string) {
	htmlLower := strings.ToLower(html)

	// Check for SPA framework shells
	spaMarkers := []struct {
		pattern string
		reason  string
	}{
		{"<div id=\"__next\"", "nextjs-shell"},
		{"<div id=\"__nuxt\"", "nuxt-shell"},
		{"<div id=\"app\"></div>", "vue-empty-shell"},
		{"<div id=\"root\"></div>", "react-empty-shell"},
		{"<noscript>you need to enable javascript", "noscript-warning"},
		{"<noscript>please enable javascript", "noscript-warning"},
		{"window.__INITIAL_STATE__", "ssr-hydration"},
	}

	for _, marker := range spaMarkers {
		if strings.Contains(htmlLower, marker.pattern) {
			// Check if there's actual content too
			// Many SSR sites have these markers but also have content
			bodyStart := strings.Index(htmlLower, "<body")
			if bodyStart > -1 {
				body := htmlLower[bodyStart:]
				// If body has very little text content, it's likely empty shell
				textContent := stripHTMLTags(body)
				if len(strings.Fields(textContent)) < 100 {
					return true, marker.reason
				}
			}
		}
	}

	// Check for bot protection pages
	botMarkers := []struct {
		pattern string
		reason  string
	}{
		{"cf-browser-verification", "cloudflare-challenge"},
		{"challenge-running", "cloudflare-challenge"},
		{"_cf_chl_opt", "cloudflare-challenge"},
		{"datadome", "datadome-protection"},
		{"perimeterx", "perimeterx-protection"},
		{"distil_r_captcha", "distil-protection"},
		{"incapsula", "incapsula-protection"},
		{"access denied", "access-denied"},
	}

	for _, marker := range botMarkers {
		if strings.Contains(htmlLower, marker.pattern) {
			return true, marker.reason
		}
	}

	return false, ""
}

func stripHTMLTags(html string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(html, " ")
}
