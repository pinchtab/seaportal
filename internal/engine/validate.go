package engine

import (
	"regexp"
	"strings"
)

type Validation struct {
	IsValid       bool     `json:"isValid"`
	NeedsBrowser  bool     `json:"needsBrowser"`
	Confidence    float64  `json:"confidence"`
	Issues        []string `json:"issues"`
	LinkDensity   float64  `json:"linkDensity"`
	SkeletonScore float64  `json:"skeletonScore"`
	ContentRatio  float64  `json:"contentRatio"`
}

var (
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

	bodyTrusted := r.Length >= 20000 && r.ParagraphCount >= 10 && r.HeadingCount >= 2

	wordCount := len(strings.Fields(r.Content))
	if wordCount < 50 {
		v.Issues = append(v.Issues, "minimal-content")
		v.Confidence -= 0.3
		if wordCount < 10 {
			v.NeedsBrowser = true
			v.IsValid = false
		}
	}

	if r.ParagraphCount > 0 {
		v.LinkDensity = float64(r.LinkCount) / float64(r.ParagraphCount)
		if v.LinkDensity > 3.0 {
			v.Issues = append(v.Issues, "nav-heavy")
			if !bodyTrusted {
				v.Confidence -= 0.2
			}
		}
		if v.LinkDensity > 5.0 && !bodyTrusted {
			v.NeedsBrowser = true
		}
	}

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

	if !bodyTrusted {
		for _, pattern := range botProtectionPatterns {
			if pattern.MatchString(content) {
				v.Issues = append(v.Issues, "bot-protection")
				v.NeedsBrowser = true
				v.IsValid = false
				v.Confidence = 0.1
				break
			}
		}
	}

	for _, pattern := range paywallPatterns {
		if pattern.MatchString(content) {
			v.Issues = append(v.Issues, "paywall-detected")
			v.Confidence -= 0.2
			break
		}
	}

	if r.HeadingCount > 0 && r.ParagraphCount > 0 {
		headingRatio := float64(r.HeadingCount) / float64(r.ParagraphCount)
		if headingRatio > 0.8 {
			v.Issues = append(v.Issues, "heading-heavy")
			v.Confidence -= 0.15
		}
	}

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

	totalElements := r.HeadingCount + r.ParagraphCount + r.LinkCount
	if totalElements > 0 {
		v.ContentRatio = float64(r.ParagraphCount) / float64(totalElements)
	}

	if v.Confidence < 0 {
		v.Confidence = 0
	}
	if v.Confidence < 0.5 {
		v.IsValid = false
	}

	return v
}

func QuickNeedsBrowser(html string) (needsBrowser bool, reason string) {
	htmlLower := strings.ToLower(html)

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
			bodyStart := strings.Index(htmlLower, "<body")
			if bodyStart > -1 {
				body := htmlLower[bodyStart:]
				textContent := stripHTMLTags(body)
				if len(strings.Fields(textContent)) < 100 {
					return true, marker.reason
				}
			}
		}
	}

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
