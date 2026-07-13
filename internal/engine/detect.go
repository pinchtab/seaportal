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
	bodyOpenRE      = regexp.MustCompile(`(?i)<body`)
	bodyCloseRE     = regexp.MustCompile(`(?i)</body>`)
	stripTagsRE     = regexp.MustCompile(`<[^>]*>`)
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
		regexp.MustCompile(`(?i)<title>\s*just\s+a\s+moment`),
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

	if loc := bodyOpenRE.FindStringIndex(html); loc != nil {
		bodyContentStart := loc[1]
		if closeLoc := bodyCloseRE.FindStringIndex(html[bodyContentStart:]); closeLoc != nil {
			bodyContent := html[bodyContentStart : bodyContentStart+closeLoc[0]]
			textOnly := strings.TrimSpace(stripTagsRE.ReplaceAllString(bodyContent, ""))
			if len(textOnly) < 200 {
				signals = append(signals, "minimal-body-content")
			}
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

func DetectJSChallenge(html string, contentType string, length int) bool {
	if !strings.Contains(strings.ToLower(contentType), "html") {
		return false
	}
	if length > 1500 || length <= 0 {
		return false
	}
	lower := strings.ToLower(html)
	challengeSignals := []string{
		"cf-mitigated",
		"__cf_bm",
		"challenge-platform",
		"datadome",
		"px-captcha",
		"perimeterx",
		"akamai-bm",
		"challenge.akamaized",
		"ddos protection by cloudflare",
		"checking your browser",
		"just a moment",
	}
	for _, sig := range challengeSignals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

func DetectBlocked(html string) bool {
	headPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<title>\s*just\s+a\s+moment`),
		regexp.MustCompile(`(?i)<title>[^<]*cloudflare`),
		regexp.MustCompile(`(?i)<title>[^<]*captcha`),
		regexp.MustCompile(`(?i)<title>[^<]*access\s+denied`),
		regexp.MustCompile(`(?i)<title>\s*client\s+challenge`),
		regexp.MustCompile(`(?i)<title>[^<]*rate\s+limit`),
		regexp.MustCompile(`(?i)window\._cf_chl_opt`),
		regexp.MustCompile(`(?i)/_px(Capt|vid|hc)`),
		regexp.MustCompile(`(?i)px-captcha`),
		regexp.MustCompile(`(?i)_incapsula_resource`),
		regexp.MustCompile(`(?i)<title>[^<]*pardon\s+our\s+interruption`),
		regexp.MustCompile(`(?i)action="[^"]*validateCaptcha`),
		regexp.MustCompile(`(?i)opfcaptcha\.amazon`),
		regexp.MustCompile(`(?i)csm-captcha-instrumentation`),
		regexp.MustCompile(`(?i)AwsWafIntegration`),
		regexp.MustCompile(`(?i)<div\s+id="challenge-container">\s*</div>`),
		regexp.MustCompile(`(?i)<script[^>]+src="/_sec/cp_challenge/`),
		regexp.MustCompile(`(?i)\bbm-verify\b`),
		regexp.MustCompile(`(?i)Reference\s+#18\.[a-f0-9]+`),
		regexp.MustCompile(`(?i)src="[^"]*dd\.datadome\.co`),
		regexp.MustCompile(`(?i)\bdatadome\b\s*[:=]`),
		regexp.MustCompile(`(?i)gddRu\s*=`),
		regexp.MustCompile(`(?i)Request\s+unsuccessful\.\s+Incapsula\s+incident\s+ID`),
	}
	for _, p := range headPatterns {
		if p.MatchString(html) {
			return true
		}
	}

	openLoc := bodyOpenRE.FindStringIndex(html)
	if openLoc == nil {
		return false
	}
	closeLoc := bodyCloseRE.FindStringIndex(html[openLoc[0]:])
	if closeLoc == nil {
		return false
	}
	bodyContent := html[openLoc[0] : openLoc[0]+closeLoc[0]]
	noScripts := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(bodyContent, "")
	noStyles := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(noScripts, "")
	textOnly := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(noStyles, "")
	textOnly = strings.TrimSpace(textOnly)

	if len(textOnly) > 1000 {
		return false
	}

	for _, p := range blockedPatterns {
		if p.MatchString(textOnly) {
			return true
		}
	}

	return false
}

type confidenceInputs struct {
	length         int
	headingCount   int
	paragraphCount int
	spaSignalCount int
	isBlocked      bool
}

func confidenceInputsFrom(r *Result) confidenceInputs {
	return confidenceInputs{
		length:         r.Length,
		headingCount:   r.HeadingCount,
		paragraphCount: r.ParagraphCount,
		spaSignalCount: len(r.SPASignals),
		isBlocked:      r.IsBlocked,
	}
}

func computeConfidence(in confidenceInputs) int {
	confidence := 100

	if in.length < 100 {
		confidence -= 50
	} else if in.length < 500 {
		confidence -= 20
	}

	if in.headingCount == 0 {
		confidence -= 10
	}

	if in.paragraphCount == 0 {
		confidence -= 15
	}

	confidence -= in.spaSignalCount * 20

	if in.isBlocked {
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

func CountMarkdownHeadings(content string) int {
	headingRe := regexp.MustCompile(`(?m)^#{1,6}\s`)
	return len(headingRe.FindAllString(content, -1))
}

func extractMarkdownTitle(content string) string {
	if fm, ok := readYAMLFrontmatter(content); ok {
		for _, line := range strings.Split(fm, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "title:") {
				continue
			}
			title := strings.TrimSpace(strings.TrimPrefix(line, "title:"))
			title = strings.Trim(title, `"'`)
			if title != "" {
				return title
			}
		}
	}
	headingRe := regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	if m := headingRe.FindStringSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func readYAMLFrontmatter(content string) (string, bool) {
	switch {
	case strings.HasPrefix(content, "---\n"):
		content = content[4:]
	case strings.HasPrefix(content, "---\r\n"):
		content = content[5:]
	default:
		return "", false
	}
	var fm strings.Builder
	for content != "" {
		nl := strings.IndexByte(content, '\n')
		var line string
		if nl < 0 {
			line = content
			content = ""
		} else {
			line = content[:nl]
			content = content[nl+1:]
		}
		if strings.TrimRight(line, "\r") == "---" {
			return fm.String(), true
		}
		if fm.Len() > 0 {
			fm.WriteByte('\n')
		}
		fm.WriteString(line)
	}
	return "", false
}

func countMarkdownParagraphs(content string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") ||
			strings.HasPrefix(line, "*") || strings.HasPrefix(line, ">") ||
			strings.HasPrefix(line, "```") || strings.HasPrefix(line, "---") {
			continue
		}
		if len(line) > 40 {
			count++
		}
	}
	return count
}

var (
	reLinkURL = regexp.MustCompile(`<([^>]+)>`)
	reLinkRel = regexp.MustCompile(`(?i)\brel\s*=\s*["']?([a-zA-Z0-9_-]+)["']?`)
)

func extractLLMsTxtURL(linkHeader string) string {
	parts := strings.Split(linkHeader, ",")

	findRel := func(want string) string {
		for _, part := range parts {
			part = strings.TrimSpace(part)
			rel := reLinkRel.FindStringSubmatch(part)
			if len(rel) < 2 || !strings.EqualFold(rel[1], want) {
				continue
			}
			if m := reLinkURL.FindStringSubmatch(part); len(m) > 1 {
				return m[1]
			}
		}
		return ""
	}

	if u := findRel("llms-full-txt"); u != "" {
		return u
	}
	return findRel("llms-txt")
}

func CountMarkdownLinks(content string) int {
	allRe := regexp.MustCompile(`\[[^\]]+\]\([^)]+\)`)
	allCount := len(allRe.FindAllString(content, -1))
	imgRe := regexp.MustCompile(`!\[[^\]]*\]\([^)]+\)`)
	imgCount := len(imgRe.FindAllString(content, -1))
	return allCount - imgCount
}
