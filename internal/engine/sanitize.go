package engine

import (
	"regexp"
)

var removeTags = map[string]bool{
	"meta":     true,
	"template": true,
	"svg":      true,
	"canvas":   true,
	"iframe":   true,
	"object":   true,
	"embed":    true,
	"noscript": true,
	"style":    true,
	"link":     true,
}

var hiddenClassNames = map[string]bool{
	"sr-only":            true,
	"visually-hidden":    true,
	"screen-reader-only": true,
}

var (
	reHTMLComments     = regexp.MustCompile(`<!--[\s\S]*?-->`)
	reInputHidden      = regexp.MustCompile(`(?i)<input\b[^>]*\btype\s*=\s*["']hidden["'][^>]*/?>`)
	reInvisibleUnicode = regexp.MustCompile("[\u200B-\u200F\u202A-\u202E\u2060-\u2064\u206A-\u206F\uFEFF]")
)

var (
	reAttrAriaHidden   = regexp.MustCompile(`(?i)\baria-hidden\s*=\s*["']true["']`)
	reAttrDisplayNone  = regexp.MustCompile(`(?i)\bstyle\s*=\s*["'][^"']*display\s*:\s*none[^"']*["']`)
	reAttrVisHidden    = regexp.MustCompile(`(?i)\bstyle\s*=\s*["'][^"']*visibility\s*:\s*hidden[^"']*["']`)
	reAttrOpacityZero  = regexp.MustCompile(`(?i)\bstyle\s*=\s*["'][^"']*opacity\s*:\s*0\s*[;"][^"']*["']`)
	reAttrFontSizeZero = regexp.MustCompile(`(?i)\bstyle\s*=\s*["'][^"']*font-size\s*:\s*0[^"']*["']`)
)

func buildClassAttrPatterns() []*regexp.Regexp {
	var patterns []*regexp.Regexp
	for cls := range hiddenClassNames {
		escapedCls := regexp.QuoteMeta(cls)
		re := regexp.MustCompile(`(?i)\bclass\s*=\s*["'][^"']*\b` + escapedCls + `\b[^"']*["']`)
		patterns = append(patterns, re)
	}
	return patterns
}

var classAttrPatterns = buildClassAttrPatterns()

func SanitizeHTML(html string) string {
	html = reHTMLComments.ReplaceAllString(html, "")

	html = removeAlwaysHiddenTagsSinglePass(html)

	html = reInputHidden.ReplaceAllString(html, "")

	html = removeHiddenElementsSinglePass(html)

	html = reInvisibleUnicode.ReplaceAllString(html, "")

	return html
}

func removeAlwaysHiddenTagsSinglePass(html string) string {
	return removeElementsSinglePass(html, func(tagName, _ string) bool {
		return removeTags[tagName]
	})
}

func hiddenAttrPredicates() []*regexp.Regexp {
	preds := []*regexp.Regexp{
		reAttrAriaHidden,
		reAttrDisplayNone,
		reAttrVisHidden,
		reAttrOpacityZero,
		reAttrFontSizeZero,
	}
	preds = append(preds, classAttrPatterns...)
	return preds
}

var hiddenPredicates = hiddenAttrPredicates()

func removeHiddenElementsSinglePass(html string) string {
	return removeElementsSinglePass(html, func(_, attrs string) bool {
		if attrs == "" {
			return false
		}
		if hasHiddenBoolAttr(attrs) {
			return true
		}
		for _, re := range hiddenPredicates {
			if re.MatchString(attrs) {
				return true
			}
		}
		return false
	})
}

func StripInvisibleUnicode(text string) string {
	return reInvisibleUnicode.ReplaceAllString(text, "")
}
