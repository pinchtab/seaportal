// Package portal provides content extraction with SPA detection
package engine

import (
	"regexp"
	"strings"
)

// CleanupMarkdown removes noise patterns from markdown output:
// - Empty links [](url) that provide no value
// - HTML comments <!--...--> that leak through
// - Standalone bullet items with only empty links
// - App download CTAs (QR codes, app store links)
func CleanupMarkdown(md string) string {
	commentRe := regexp.MustCompile(`<!--[^>]*-->`)
	md = commentRe.ReplaceAllString(md, "")

	// Remove standalone list items that are just empty links
	// Matches: - [](url) on its own line
	emptyLinkListRe := regexp.MustCompile(`(?m)^[\-\*\+]\s*\[\]\([^)]+\)\s*$`)
	md = emptyLinkListRe.ReplaceAllString(md, "")

	emptyLinkRe := regexp.MustCompile(`\[\]\([^)]+\)`)
	md = emptyLinkRe.ReplaceAllString(md, "")

	appCtaRe := regexp.MustCompile(`(?im)^.*(?:scan the qr code|download the.*app|get the app|available on.*app store|available on.*google play).*$\n?`)
	md = appCtaRe.ReplaceAllString(md, "")

	multiBlankRe := regexp.MustCompile(`\n{3,}`)
	md = multiBlankRe.ReplaceAllString(md, "\n\n")

	return strings.TrimSpace(md)
}
