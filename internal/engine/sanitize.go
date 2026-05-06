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

// removeAttrElements removes elements (and their content) whose opening tag
// matches attrPattern. Walks the input once, tracking nesting depth so the
// close tag belongs to the matched opening tag — naive `strings.Index` of
// `</tag` would match the first inner close on nested same-tag markup and
// leave the outer close behind as broken HTML.
func removeAttrElements(html string, attrPattern *regexp.Regexp) string {
	reOpenTag := regexp.MustCompile(`(?i)<([a-z][a-z0-9]*)\b[^>]*` + attrPattern.String() + `[^>]*>`)

	var sb strings.Builder
	sb.Grow(len(html))

	pos := 0
	for pos < len(html) {
		loc := reOpenTag.FindStringSubmatchIndex(html[pos:])
		if loc == nil {
			sb.WriteString(html[pos:])
			return sb.String()
		}
		matchStart := pos + loc[0]
		matchEnd := pos + loc[1]
		tagName := strings.ToLower(html[pos+loc[2] : pos+loc[3]])

		sb.WriteString(html[pos:matchStart])

		// Self-closing (`<tag … />`) or void element: no close tag to find.
		if (matchEnd-matchStart >= 2 && html[matchEnd-2] == '/') || isVoidElement(tagName) {
			pos = matchEnd
			continue
		}

		closeEnd := findMatchingClose(html, matchEnd, tagName)
		if closeEnd < 0 {
			// Unclosed: drop just the opening tag.
			pos = matchEnd
			continue
		}
		pos = closeEnd
	}

	return sb.String()
}

// findMatchingClose returns the absolute index just past the '>' of the
// </tagName> close that balances an opening tag at openEnd, or -1 if no
// balanced close exists before end-of-input.
//
// Tracks nesting depth so `<div><div>x</div></div>` resolves to the outer
// close. Self-closing forms (`<tag/>`) do not push depth. Quoted attribute
// values are skipped so `>` characters inside attributes are ignored.
func findMatchingClose(html string, openEnd int, tagName string) int {
	depth := 1
	i := openEnd
	n := len(html)
	for i < n {
		lt := strings.IndexByte(html[i:], '<')
		if lt < 0 {
			return -1
		}
		i += lt
		if i+1 >= n {
			return -1
		}
		if html[i+1] == '/' {
			if hasTagPrefix(html[i+2:], tagName) {
				gt := strings.IndexByte(html[i:], '>')
				if gt < 0 {
					return -1
				}
				depth--
				if depth == 0 {
					return i + gt + 1
				}
				i += gt + 1
				continue
			}
			gt := strings.IndexByte(html[i:], '>')
			if gt < 0 {
				return -1
			}
			i += gt + 1
			continue
		}
		if hasTagPrefix(html[i+1:], tagName) {
			tagEnd, selfClosing := scanTagEnd(html, i)
			if tagEnd < 0 {
				return -1
			}
			if !selfClosing && !isVoidElement(tagName) {
				depth++
			}
			i = tagEnd
			continue
		}
		gt := strings.IndexByte(html[i:], '>')
		if gt < 0 {
			return -1
		}
		i += gt + 1
	}
	return -1
}

// hasTagPrefix reports whether s begins with tagName followed by an HTML
// tag-name terminator (whitespace, '/', or '>'). Case-insensitive.
func hasTagPrefix(s, tagName string) bool {
	if len(s) < len(tagName) {
		return false
	}
	if !strings.EqualFold(s[:len(tagName)], tagName) {
		return false
	}
	if len(s) == len(tagName) {
		return true
	}
	return isAttrTerminator(s[len(tagName)])
}

// scanTagEnd returns the absolute index just past the '>' of the tag opening
// at i, and whether the tag is self-closing ('/>'). Skips '>' bytes inside
// quoted attribute values.
func scanTagEnd(html string, i int) (int, bool) {
	j := i + 1
	n := len(html)
	for j < n {
		switch c := html[j]; c {
		case '"', '\'':
			k := strings.IndexByte(html[j+1:], c)
			if k < 0 {
				return -1, false
			}
			j += k + 2
		case '>':
			selfClosing := j > i+1 && html[j-1] == '/'
			return j + 1, selfClosing
		default:
			j++
		}
	}
	return -1, false
}

// HTML void elements have no closing tag.
// https://html.spec.whatwg.org/multipage/syntax.html#void-elements
var voidElements = map[string]bool{
	"area":   true,
	"base":   true,
	"br":     true,
	"col":    true,
	"embed":  true,
	"hr":     true,
	"img":    true,
	"input":  true,
	"link":   true,
	"meta":   true,
	"param":  true,
	"source": true,
	"track":  true,
	"wbr":    true,
}

func isVoidElement(tag string) bool {
	return voidElements[tag]
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

// removeHiddenAttrElements removes elements with the HTML hidden boolean
// attribute. Uses a quote-aware attribute scanner so "hidden" inside
// attribute values (e.g. class="visually-hidden") is not matched, and
// findMatchingClose so nested same-tag markup resolves to the right close.
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

		out.WriteString(html[pos:matchStart])

		if (matchEnd-matchStart >= 2 && html[matchEnd-2] == '/') || isVoidElement(tagName) {
			pos = matchEnd
			continue
		}

		closeEnd := findMatchingClose(html, matchEnd, tagName)
		if closeEnd < 0 {
			pos = matchEnd
			continue
		}
		pos = closeEnd
	}
	return out.String()
}

// StripInvisibleUnicode removes zero-width and invisible Unicode characters
// from extracted text content.
func StripInvisibleUnicode(text string) string {
	return reInvisibleUnicode.ReplaceAllString(text, "")
}
