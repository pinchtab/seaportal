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
	//
	// performance: the previous implementation ran one regex per tag,
	// each `(?is)<TAG\b[^>]*(?:/>|>[\s\S]*?</TAG\s*>)`. On a 1.3 MB
	// Wikipedia fixture (which contains none of these tags) the NFA
	// re-scanned the full document for each tag, accounting for >85%
	// of total CPU in pprof. Replaced with a single-pass tokenizer.
	html = removeAlwaysHiddenTagsSinglePass(html)

	// 3. Remove input[type=hidden].
	html = reInputHidden.ReplaceAllString(html, "")

	// 4-7. Remove elements whose opening tag matches ANY of the hidden
	// predicates (aria-hidden, hidden bool attr, hidden classes, hidden
	// inline styles) in a single pass over the document.
	//
	// performance: the previous implementation invoked removeAttrElements
	// once per predicate. Each call ran a regex over the full remaining
	// document inside a per-tag loop — on a 1.3 MB Wikipedia fixture the
	// combined regex backtracking dominated wall time (>87% of CPU per
	// pprof, ~13s/op). The single-pass scanner below tokenises the HTML
	// once and tests each opening tag's attribute substring against the
	// (already compiled) predicates locally, dropping the cost from
	// O(N_predicates · M_tags · D_doc) to O(D_doc).
	html = removeHiddenElementsSinglePass(html)

	// 8. Strip invisible Unicode characters.
	html = reInvisibleUnicode.ReplaceAllString(html, "")

	return html
}

// removeAlwaysHiddenTagsSinglePass scans the HTML once and drops every
// element whose tag name is in removeTags (svg, canvas, style, meta,
// template, iframe, object, embed, noscript, link). Self-closing and void
// forms are removed without seeking a close tag; otherwise the matching
// `</tag>` (with depth tracking) is consumed too.
func removeAlwaysHiddenTagsSinglePass(html string) string {
	var out strings.Builder
	out.Grow(len(html))
	pos := 0
	n := len(html)
	for pos < n {
		lt := strings.IndexByte(html[pos:], '<')
		if lt < 0 {
			out.WriteString(html[pos:])
			break
		}
		tagStartAbs := pos + lt
		if tagStartAbs+1 >= n {
			out.WriteString(html[pos:])
			break
		}
		c := html[tagStartAbs+1]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			// Not a candidate opening tag — copy `<` and continue.
			out.WriteString(html[pos : tagStartAbs+1])
			pos = tagStartAbs + 1
			continue
		}
		nameStart := tagStartAbs + 1
		nameEnd := nameStart
		for nameEnd < n {
			ch := html[nameEnd]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
				nameEnd++
				continue
			}
			break
		}
		if nameEnd == nameStart {
			out.WriteString(html[pos : tagStartAbs+1])
			pos = tagStartAbs + 1
			continue
		}
		tagEnd, selfClosing := scanTagEnd(html, tagStartAbs)
		if tagEnd < 0 {
			out.WriteString(html[pos:])
			break
		}
		tagName := strings.ToLower(html[nameStart:nameEnd])
		if !removeTags[tagName] {
			out.WriteString(html[pos:tagEnd])
			pos = tagEnd
			continue
		}
		// Always-hidden tag — drop opening (and subtree if applicable).
		out.WriteString(html[pos:tagStartAbs])
		if selfClosing || isVoidElement(tagName) {
			pos = tagEnd
			continue
		}
		closeEnd := findMatchingClose(html, tagEnd, tagName)
		if closeEnd < 0 {
			pos = tagEnd
			continue
		}
		pos = closeEnd
	}
	return out.String()
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

// removeHiddenElementsSinglePass walks the HTML once, locates every opening
// tag, and removes the element when its attribute string matches any hidden
// predicate OR carries a standalone `hidden` boolean attribute. Uses the
// existing depth-tracking findMatchingClose so nested same-tag markup is
// resolved correctly.
func removeHiddenElementsSinglePass(html string) string {
	var out strings.Builder
	out.Grow(len(html))
	pos := 0
	n := len(html)
	for pos < n {
		lt := strings.IndexByte(html[pos:], '<')
		if lt < 0 {
			out.WriteString(html[pos:])
			break
		}
		tagStartAbs := pos + lt

		// Only consider opening tags `<[a-z]...>`. Skip `</`, `<!`, `<?`.
		if tagStartAbs+1 >= n {
			out.WriteString(html[pos:])
			break
		}
		c := html[tagStartAbs+1]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			// Not an opening tag we care about; copy `<` and continue.
			out.WriteString(html[pos : tagStartAbs+1])
			pos = tagStartAbs + 1
			continue
		}

		// Scan tag name.
		nameStart := tagStartAbs + 1
		nameEnd := nameStart
		for nameEnd < n {
			ch := html[nameEnd]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
				nameEnd++
				continue
			}
			break
		}
		if nameEnd == nameStart {
			out.WriteString(html[pos : tagStartAbs+1])
			pos = tagStartAbs + 1
			continue
		}

		// Find tag end (handles quoted attributes).
		tagEnd, selfClosing := scanTagEnd(html, tagStartAbs)
		if tagEnd < 0 {
			out.WriteString(html[pos:])
			break
		}

		attrs := ""
		if nameEnd < tagEnd-1 {
			attrs = html[nameEnd : tagEnd-1]
			// Trim trailing '/' for self-closing.
			if selfClosing && len(attrs) > 0 && attrs[len(attrs)-1] == '/' {
				attrs = attrs[:len(attrs)-1]
			}
		}

		matched := false
		if attrs != "" {
			if hasHiddenBoolAttr(attrs) {
				matched = true
			} else {
				for _, re := range hiddenPredicates {
					if re.MatchString(attrs) {
						matched = true
						break
					}
				}
			}
		}

		if !matched {
			out.WriteString(html[pos:tagEnd])
			pos = tagEnd
			continue
		}

		// Element is hidden — drop it (and its subtree if not self-closing/void).
		tagName := strings.ToLower(html[nameStart:nameEnd])
		out.WriteString(html[pos:tagStartAbs])

		if selfClosing || isVoidElement(tagName) {
			pos = tagEnd
			continue
		}
		closeEnd := findMatchingClose(html, tagEnd, tagName)
		if closeEnd < 0 {
			// Unclosed: drop just the opening tag.
			pos = tagEnd
			continue
		}
		pos = closeEnd
	}
	return out.String()
}

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

// StripInvisibleUnicode removes zero-width and invisible Unicode characters
// from extracted text content.
func StripInvisibleUnicode(text string) string {
	return reInvisibleUnicode.ReplaceAllString(text, "")
}
