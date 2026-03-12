package portal

import (
	"regexp"
	"strings"
)

// PreprocessHTML normalizes HTML before parsing to preserve content that would
// otherwise be stripped by go-readability or html-to-markdown converters.
func PreprocessHTML(html string) string {
	// Replace twoslash buttons with spans to preserve code content
	// Pattern: <button class="twoslash-hover" ...>content</button>
	// These are used in Next.js sites (nodejs.org) for syntax-highlighted code with type tooltips
	html = replaceTwoslashButtons(html)

	// VitePress/VuePress content scoping: extract main content container
	// to prevent readability from selecting nav dropdowns over main content
	html = scopeVitePressContent(html)

	return html
}

var twoslashButtonPattern = regexp.MustCompile(`<button\s+[^>]*class=["'][^"']*twoslash[^"']*["'][^>]*>(.*?)</button>`)

// replaceTwoslashButtons converts <button class="twoslash-hover">content</button> to <span>content</span>
// This preserves variable names and code content that would otherwise be stripped
func replaceTwoslashButtons(html string) string {
	return twoslashButtonPattern.ReplaceAllString(html, "<span>$1</span>")
}

var vitePressDetectPattern = regexp.MustCompile(`class=["'][^"']*\bVPApp\b|id=["']VPContent["']`)
var vitePressFooterPattern = regexp.MustCompile(`(?is)<section[^>]*id=["']sitemap["'][^>]*>.*?</section>`)

// scopeVitePressContent extracts the main content container from VitePress/VuePress sites
// to prevent go-readability from selecting navigation menus as the main content
func scopeVitePressContent(html string) string {
	if !vitePressDetectPattern.MatchString(html) {
		return html
	}

	// Find and extract VPContent container
	// The pattern captures everything inside <... id="VPContent"...>...</...>
	startIdx := strings.Index(html, `id="VPContent"`)
	if startIdx == -1 {
		startIdx = strings.Index(html, `id='VPContent'`)
	}
	if startIdx == -1 {
		return html
	}

	tagStart := strings.LastIndex(html[:startIdx], "<")
	if tagStart == -1 {
		return html
	}

	tagEnd := strings.Index(html[startIdx:], ">")
	if tagEnd == -1 {
		return html
	}
	contentStart := startIdx + tagEnd + 1

	tagName := extractTagName(html[tagStart:startIdx])
	if tagName == "" {
		return html
	}

	// Find the closing tag - look for </tagname> starting from contentStart
	// Use simple depth tracking for nested tags
	content := extractNestedContent(html[contentStart:], tagName)
	if content == "" {
		return html
	}

	// Strip VitePress footer/sitemap sections (navigation links that confuse extraction)
	content = vitePressFooterPattern.ReplaceAllString(content, "")

	vpFooterPattern := regexp.MustCompile(`(?is)<div[^>]*class=["'][^"']*VPFooter[^"']*["'][^>]*>.*?</div>`)
	content = vpFooterPattern.ReplaceAllString(content, "")

	// Wrap extracted content in minimal HTML structure
	// Preserve head for meta info but replace body with just VPContent
	headEnd := strings.Index(html, "</head>")
	if headEnd == -1 {
		return "<html><body><main>" + content + "</main></body></html>"
	}
	head := html[:headEnd+7]
	return head + "<body><main id=\"VPContent\">" + content + "</main></body></html>"
}

// extractTagName extracts the tag name from an opening tag fragment like "<div " or "<section "
func extractTagName(fragment string) string {
	fragment = strings.TrimPrefix(fragment, "<")
	fragment = strings.TrimLeft(fragment, " \t\n")
	for i, c := range fragment {
		if c == ' ' || c == '\t' || c == '\n' || c == '>' || c == '/' {
			return strings.ToLower(fragment[:i])
		}
	}
	return strings.ToLower(fragment)
}

// extractNestedContent extracts content up to the matching closing tag, handling nesting
func extractNestedContent(html string, tagName string) string {
	openTag := "<" + tagName
	closeTag := "</" + tagName
	depth := 1
	i := 0

	for i < len(html) && depth > 0 {
		nextOpen := strings.Index(strings.ToLower(html[i:]), openTag)
		nextClose := strings.Index(strings.ToLower(html[i:]), closeTag)

		if nextClose == -1 {
			return ""
		}

		if nextOpen != -1 && nextOpen < nextClose {
			// Found opening tag first - check if it's actually a tag (has space, > or />)
			tagEndPos := nextOpen + len(openTag)
			if tagEndPos < len(html[i:]) {
				nextChar := html[i:][tagEndPos]
				if nextChar == ' ' || nextChar == '>' || nextChar == '\t' || nextChar == '\n' || nextChar == '/' {
					depth++
				}
			}
			i += nextOpen + 1
		} else {
			depth--
			if depth == 0 {
				return html[:i+nextClose]
			}
			i += nextClose + 1
		}
	}

	return ""
}
