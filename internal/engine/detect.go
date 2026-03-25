package engine

import (
	"regexp"
	"strings"
)

var (
	jsRequiredPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)javascript\s+(is\s+)?(required|disabled|not\s+enabled)`),
		regexp.MustCompile(`(?i)please\s+enable\s+javascript`),
		regexp.MustCompile(`(?i)this\s+app\s+requires\s+javascript`),
		regexp.MustCompile(`(?i)you\s+need\s+to\s+enable\s+javascript`),
	}
	spaRootPatterns = []*regexp.Regexp{
		regexp.MustCompile(`<div\s+id=["']root["']\s*>`),
		regexp.MustCompile(`<div\s+id=["']app["']\s*>`),
		regexp.MustCompile(`<div\s+id=["']__next["']\s*>`),
	}
	blockedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)captcha`),
		regexp.MustCompile(`(?i)are\s+you\s+a\s+(human|robot)`),
		regexp.MustCompile(`(?i)verify\s+you('re|\s+are)\s+(human|not\s+a\s+robot)`),
		regexp.MustCompile(`(?i)access\s+(denied|blocked)`),
		regexp.MustCompile(`(?i)bot\s+(detected|protection)`),
		regexp.MustCompile(`(?i)please\s+complete\s+the\s+security\s+check`),
		regexp.MustCompile(`(?i)cloudflare`),
		regexp.MustCompile(`(?i)ddos[\s-]protection`),
		regexp.MustCompile(`(?i)checking\s+your\s+browser`),
		regexp.MustCompile(`(?i)too\s+many\s+requests`),
		regexp.MustCompile(`(?i)rate\s+limit`),
		regexp.MustCompile(`(?i)<title>\s*just\s+a\s+moment`), // Cloudflare challenge page
	}
)

func DetectSPA(html string) (signals []string, isSPA bool) {
	for _, p := range jsRequiredPatterns {
		if p.MatchString(html) {
			signals = append(signals, "js-required-message")
			break
		}
	}

	for _, p := range spaRootPatterns {
		if p.MatchString(html) {
			signals = append(signals, "spa-root-element")
			break
		}
	}

	bodyStart := strings.Index(strings.ToLower(html), "<body")
	bodyEnd := strings.Index(strings.ToLower(html), "</body>")
	if bodyStart > 0 && bodyEnd > bodyStart {
		bodyContent := html[bodyStart:bodyEnd]
		textOnly := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(bodyContent, "")
		textOnly = strings.TrimSpace(textOnly)
		if len(textOnly) < 200 {
			signals = append(signals, "minimal-body-content")
		}
	}

	noscriptMatch := regexp.MustCompile(`(?is)<noscript[^>]*>(.*?)</noscript>`).FindStringSubmatch(html)
	if len(noscriptMatch) > 1 {
		noscriptContent := strings.ToLower(noscriptMatch[1])
		if strings.Contains(noscriptContent, "javascript") || strings.Contains(noscriptContent, "enable") {
			signals = append(signals, "noscript-warning")
		}
	}

	isSPA = len(signals) >= 2
	return
}

// DetectBlocked: triggers on challenge pages (Cloudflare, captcha, etc.)
// Strategy: check title/head indicators first (reliable), then body text if short
func DetectBlocked(html string) bool {
	// First: check reliable head-level patterns (always check these)
	headPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<title>\s*just\s+a\s+moment`),               // Cloudflare challenge
		regexp.MustCompile(`(?i)<title>[^<]*cloudflare`),                    // Cloudflare in title
		regexp.MustCompile(`(?i)<title>[^<]*captcha`),                       // Captcha in title
		regexp.MustCompile(`(?i)<title>[^<]*access\s+denied`),               // Access denied in title
		regexp.MustCompile(`(?i)<title>\s*client\s+challenge`),              // Generic client challenge page
		regexp.MustCompile(`(?i)<title>[^<]*rate\s+limit`),                  // Rate limit in title
		regexp.MustCompile(`(?i)window\._cf_chl_opt`),                       // Cloudflare challenge JS
		regexp.MustCompile(`(?i)/_px(Capt|vid|hc)`),                         // PerimeterX challenge
		regexp.MustCompile(`(?i)px-captcha`),                                // PerimeterX captcha widget
		regexp.MustCompile(`(?i)_incapsula_resource`),                       // Imperva/Incapsula challenge
		regexp.MustCompile(`(?i)<title>[^<]*pardon\s+our\s+interruption`),   // Incapsula block page
		regexp.MustCompile(`(?i)action="[^"]*validateCaptcha`),              // Amazon captcha form
		regexp.MustCompile(`(?i)opfcaptcha\.amazon`),                        // Amazon captcha server
		regexp.MustCompile(`(?i)csm-captcha-instrumentation`),               // Amazon captcha script
		regexp.MustCompile(`(?i)AwsWafIntegration`),                         // AWS WAF JS challenge
		regexp.MustCompile(`(?i)<div\s+id="challenge-container">\s*</div>`), // AWS WAF challenge container
	}
	for _, p := range headPatterns {
		if p.MatchString(html) {
			return true
		}
	}

	// Second: check body content for blocked indicators (only if body is short)
	htmlLower := strings.ToLower(html)
	bodyStart := strings.Index(htmlLower, "<body")
	bodyEnd := strings.Index(htmlLower, "</body>")
	if bodyStart > 0 && bodyEnd > bodyStart {
		bodyContent := html[bodyStart:bodyEnd]
		// Strip both tags AND script content before measuring
		noScripts := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(bodyContent, "")
		noStyles := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(noScripts, "")
		textOnly := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(noStyles, "")
		textOnly = strings.TrimSpace(textOnly)

		// Only check body patterns on short pages to avoid false positives
		if len(textOnly) > 1000 {
			return false
		}

		// Check blocked patterns against the stripped body text (not full HTML)
		// This avoids false positives from "captcha" in scripts/configs
		for _, p := range blockedPatterns {
			if p.MatchString(textOnly) {
				return true
			}
		}
	}

	return false
}

func ComputeConfidence(length, headingCount, paragraphCount, spaSignalCount int, isBlocked bool) int {
	confidence := 100

	if length < 100 {
		confidence -= 50
	} else if length < 500 {
		confidence -= 20
	}

	if headingCount == 0 {
		confidence -= 10
	}

	if paragraphCount == 0 {
		confidence -= 15
	}

	confidence -= spaSignalCount * 20

	if isBlocked {
		confidence -= 30
	}

	if confidence < 0 {
		confidence = 0
	}
	return confidence
}

func CountPattern(html string, pattern string) int {
	re := regexp.MustCompile(pattern)
	return len(re.FindAllString(html, -1))
}

// CountMarkdownLinks counts Markdown-style links [text](url) in content.
// Useful for React/SPA pages where links appear in converted markdown but not raw HTML.
func CountMarkdownLinks(content string) int {
	allRe := regexp.MustCompile(`\[[^\]]+\]\([^)]+\)`)
	allCount := len(allRe.FindAllString(content, -1))
	// Subtract images ![alt](src)
	imgRe := regexp.MustCompile(`!\[[^\]]*\]\([^)]+\)`)
	imgCount := len(imgRe.FindAllString(content, -1))
	return allCount - imgCount
}
