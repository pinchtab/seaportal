package engine

import (
	"regexp"
	"strings"
)

// sanitize.go — Pre-readability HTML sanitization.
// Strips hidden elements, invisible content, and junk tags before extraction.
// Ported from OpenClaw's web-fetch-visibility.ts.

// Tags that should always be removed (never contain useful readable content).
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

// buildRemoveTagRe builds a regex to remove a specific HTML tag and its contents.
// Uses the specific tag name in both open and close tags (no backreferences).
func buildRemoveTagRe(tag string) *regexp.Regexp {
	return regexp.MustCompile(`(?is)<` + tag + `\b[^>]*(?:/>|>[\s\S]*?</` + tag + `\s*>)`)
}

// Pre-compiled removal patterns for always-hidden tags (built once).
var tagRemovePatterns []*regexp.Regexp

func init() {
	for tag := range removeTags {
		tagRemovePatterns = append(tagRemovePatterns, buildRemoveTagRe(tag))
	}
}

// removeAttrElements removes elements where an opening tag contains a specific attribute pattern.
// Since Go regexp doesn't support backreferences, we find the opening tag, extract the tag name,
// then find the matching closing tag. Collects all ranges first, then removes in one pass.
func removeAttrElements(html string, attrPattern *regexp.Regexp) string {
	reOpenTag := regexp.MustCompile(`(?i)<([a-z][a-z0-9]*)\b[^>]*` + attrPattern.String() + `[^>]*>`)

	// Quick check: if the pattern can't possibly match, skip.
	matches := reOpenTag.FindAllStringSubmatchIndex(html, -1)
	if len(matches) == 0 {
		return html
	}

	// Collect removal ranges (start, end) — process from last to first to avoid index shifts.
	type removeRange struct{ start, end int }
	var ranges []removeRange

	for _, loc := range matches {
		tagName := strings.ToLower(html[loc[2]:loc[3]])
		openEnd := loc[1]

		closeTag := "</" + tagName
		closeIdx := strings.Index(strings.ToLower(html[openEnd:]), closeTag)
		if closeIdx == -1 {
			ranges = append(ranges, removeRange{loc[0], openEnd})
			continue
		}

		closeAbsIdx := openEnd + closeIdx
		closeEnd := strings.Index(html[closeAbsIdx:], ">")
		if closeEnd == -1 {
			closeEnd = len(html[closeAbsIdx:])
		}
		ranges = append(ranges, removeRange{loc[0], closeAbsIdx + closeEnd + 1})
	}

	if len(ranges) == 0 {
		return html
	}

	// Build result by copying non-removed segments.
	var sb strings.Builder
	sb.Grow(len(html))
	pos := 0
	for _, r := range ranges {
		if r.start < pos {
			continue // Overlapping range, skip.
		}
		sb.WriteString(html[pos:r.start])
		pos = r.end
	}
	sb.WriteString(html[pos:])
	return sb.String()
}

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
	// 1. Strip HTML comments.
	html = reHTMLComments.ReplaceAllString(html, "")

	// 2. Remove always-hidden tags (svg, canvas, style, meta, template, etc.).
	for _, re := range tagRemovePatterns {
		html = re.ReplaceAllString(html, "")
	}

	// 3. Remove input[type=hidden].
	html = reInputHidden.ReplaceAllString(html, "")

	// 4. Remove aria-hidden="true" elements.
	html = removeAttrElements(html, reAttrAriaHidden)

	// 5. Remove elements with hidden attribute (but not "hidden" as part of another attr value).
	// Only target elements where hidden is a standalone attribute, not inside a value.
	html = removeHiddenAttrElements(html)

	// 6. Remove elements with hidden class names.
	for _, re := range classAttrPatterns {
		html = removeAttrElements(html, re)
	}

	// 7. Remove elements with hidden inline styles.
	for _, re := range []*regexp.Regexp{reAttrDisplayNone, reAttrVisHidden, reAttrOpacityZero, reAttrFontSizeZero} {
		html = removeAttrElements(html, re)
	}

	// 8. Strip invisible Unicode characters.
	html = reInvisibleUnicode.ReplaceAllString(html, "")

	return html
}

// reOpenTag matches an opening tag and captures the tag name and attribute string.
var reOpenTag = regexp.MustCompile(`(?i)<([a-z][a-z0-9]*)(\s[^>]*)?>`)

// hasHiddenBoolAttr reports whether the attribute string contains a standalone
// `hidden` boolean attribute. Quoted values are skipped so that class names
// like "visually-hidden" or "ssrcss-...-VisuallyHidden" are not matched.
func hasHiddenBoolAttr(attrs string) bool {
	i := 0
	for i < len(attrs) {
		c := attrs[i]
		switch c {
		case '"', '\'':
			// skip over quoted value
			j := strings.IndexByte(attrs[i+1:], c)
			if j < 0 {
				return false
			}
			i += j + 2
		case ' ', '\t', '\n', '\r', '/':
			i++
		default:
			// read attribute name
			start := i
			for i < len(attrs) && !isAttrTerminator(attrs[i]) && attrs[i] != '=' {
				i++
			}
			name := strings.ToLower(attrs[start:i])
			for i < len(attrs) && (attrs[i] == ' ' || attrs[i] == '\t' || attrs[i] == '\n' || attrs[i] == '\r') {
				i++
			}
			if i < len(attrs) && attrs[i] == '=' {
				i++
				for i < len(attrs) && (attrs[i] == ' ' || attrs[i] == '\t') {
					i++
				}
				if i < len(attrs) && (attrs[i] == '"' || attrs[i] == '\'') {
					q := attrs[i]
					j := strings.IndexByte(attrs[i+1:], q)
					if j < 0 {
						return false
					}
					i += j + 2
				} else {
					for i < len(attrs) && !isAttrTerminator(attrs[i]) {
						i++
					}
				}
			}
			if name == "hidden" {
				return true
			}
		}
	}
	return false
}

func isAttrTerminator(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '/' || b == '>'
}

// removeHiddenAttrElements removes elements with the HTML hidden attribute.
// Uses a quote-aware attribute scanner to avoid matching "hidden" inside
// attribute values (e.g. class="visually-hidden").
func removeHiddenAttrElements(html string) string {
	var out strings.Builder
	out.Grow(len(html))
	pos := 0
	for pos < len(html) {
		loc := reOpenTag.FindStringSubmatchIndex(html[pos:])
		if loc == nil {
			out.WriteString(html[pos:])
			break
		}
		matchStart := pos + loc[0]
		matchEnd := pos + loc[1]
		tagStart := pos + loc[2]
		tagEnd := pos + loc[3]

		var attrs string
		if loc[4] >= 0 {
			attrs = html[pos+loc[4] : pos+loc[5]]
		}

		if !hasHiddenBoolAttr(attrs) {
			out.WriteString(html[pos:matchEnd])
			pos = matchEnd
			continue
		}

		tagName := strings.ToLower(html[tagStart:tagEnd])
		closeTag := "</" + tagName
		closeIdx := strings.Index(strings.ToLower(html[matchEnd:]), closeTag)
		if closeIdx == -1 {
			// Void or unclosed tag — drop just the open tag.
			out.WriteString(html[pos:matchStart])
			pos = matchEnd
			continue
		}
		closeAbsIdx := matchEnd + closeIdx
		closeGtIdx := strings.Index(html[closeAbsIdx:], ">")
		if closeGtIdx == -1 {
			out.WriteString(html[pos:matchStart])
			break
		}
		out.WriteString(html[pos:matchStart])
		pos = closeAbsIdx + closeGtIdx + 1
	}
	return out.String()
}

// StripInvisibleUnicode removes zero-width and invisible Unicode characters
// from extracted text content.
func StripInvisibleUnicode(text string) string {
	return reInvisibleUnicode.ReplaceAllString(text, "")
}
