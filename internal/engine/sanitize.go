package engine

import (
	"regexp"
)

// Pre-readability HTML sanitization: strips hidden elements, invisible content,
// and junk tags before extraction. Ported from OpenClaw's web-fetch-visibility.ts.
// Policy (which tags/attributes count as hidden) lives here; the single-pass
// tokenizer primitives live in html_scan.go.

// removeTags never contain useful readable content, so they are always dropped.
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

// Class names that indicate visually hidden content. Limited to unambiguous
// a11y/screen-reader patterns — generic names like "hidden", "invisible",
// "d-none", "offscreen", or "clip" are commonly toggled by JS as state, so
// stripping them at the HTML stage removes legitimate content (creepjs etc.).
var hiddenClassNames = map[string]bool{
	"sr-only":            true,
	"visually-hidden":    true,
	"screen-reader-only": true,
}

// Regex patterns for tag-based sanitization (used in SanitizeHTML).
var (
	reHTMLComments     = regexp.MustCompile(`<!--[\s\S]*?-->`)
	reInputHidden      = regexp.MustCompile(`(?i)<input\b[^>]*\btype\s*=\s*["']hidden["'][^>]*/?>`)
	reInvisibleUnicode = regexp.MustCompile("[\u200B-\u200F\u202A-\u202E\u2060-\u2064\u206A-\u206F\uFEFF]")
)

// Pre-compiled attribute patterns for element removal.
var (
	reAttrAriaHidden   = regexp.MustCompile(`(?i)\baria-hidden\s*=\s*["']true["']`)
	reAttrDisplayNone  = regexp.MustCompile(`(?i)\bstyle\s*=\s*["'][^"']*display\s*:\s*none[^"']*["']`)
	reAttrVisHidden    = regexp.MustCompile(`(?i)\bstyle\s*=\s*["'][^"']*visibility\s*:\s*hidden[^"']*["']`)
	reAttrOpacityZero  = regexp.MustCompile(`(?i)\bstyle\s*=\s*["'][^"']*opacity\s*:\s*0\s*[;"][^"']*["']`)
	reAttrFontSizeZero = regexp.MustCompile(`(?i)\bstyle\s*=\s*["'][^"']*font-size\s*:\s*0[^"']*["']`)
)

// buildClassAttrPatterns creates patterns for each hidden class name.
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

// SanitizeHTML removes hidden elements, junk tags, and invisible content from HTML
// before passing it to the readability extractor. This significantly improves extraction
// quality on complex pages like StackOverflow and NYTimes.
func SanitizeHTML(html string) string {
	html = reHTMLComments.ReplaceAllString(html, "")

	// performance: the previous implementation ran one regex per tag,
	// each `(?is)<TAG\b[^>]*(?:/>|>[\s\S]*?</TAG\s*>)`. On a 1.3 MB
	// Wikipedia fixture (which contains none of these tags) the NFA
	// re-scanned the full document for each tag, accounting for >85%
	// of total CPU in pprof. Replaced with a single-pass tokenizer.
	html = removeAlwaysHiddenTagsSinglePass(html)

	html = reInputHidden.ReplaceAllString(html, "")

	// performance: the previous implementation invoked removeAttrElements
	// once per predicate. Each call ran a regex over the full remaining
	// document inside a per-tag loop — on a 1.3 MB Wikipedia fixture the
	// combined regex backtracking dominated wall time (>87% of CPU per
	// pprof, ~13s/op). The single-pass scanner below tokenises the HTML
	// once and tests each opening tag's attribute substring against the
	// (already compiled) predicates locally, dropping the cost from
	// O(N_predicates · M_tags · D_doc) to O(D_doc).
	html = removeHiddenElementsSinglePass(html)

	html = reInvisibleUnicode.ReplaceAllString(html, "")

	return html
}

// removeAlwaysHiddenTagsSinglePass drops every element whose tag name is in
// removeTags (svg, canvas, style, meta, template, iframe, object, embed,
// noscript, link) in one scan.
func removeAlwaysHiddenTagsSinglePass(html string) string {
	return removeElementsSinglePass(html, func(tagName, _ string) bool {
		return removeTags[tagName]
	})
}

// hiddenAttrPredicates returns every compiled regex used to flag an opening
// tag's attribute string as "this element should be removed". Order is
// irrelevant — first match wins.
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

// removeHiddenElementsSinglePass removes every element whose attribute string
// matches a hidden predicate OR carries a standalone `hidden` boolean
// attribute, in one scan.
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

// StripInvisibleUnicode removes zero-width and invisible Unicode characters
// from extracted text content.
func StripInvisibleUnicode(text string) string {
	return reInvisibleUnicode.ReplaceAllString(text, "")
}
