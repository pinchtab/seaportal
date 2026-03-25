// Package portal provides content extraction with SPA detection
package engine

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// TextFallbackResult holds results from text-based fallback extraction
type TextFallbackResult struct {
	Content  string
	Length   int
	Headings int
	Links    int
}

// TextFallback extracts visible text content directly from HTML when readability fails.
// This is a last-resort extraction that finds all text nodes in the body,
// skipping script/style/nav/footer elements, and formats them as markdown.
func TextFallback(htmlStr string) TextFallbackResult {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return TextFallbackResult{}
	}

	var sections []string
	var headings int
	var links int

	// Skip these elements entirely
	skipTags := map[string]bool{
		"script": true, "style": true, "noscript": true,
		"nav": true, "footer": true, "header": true,
		"svg": true, "iframe": true, "form": true,
	}

	// Skip elements with these classes/roles
	skipClasses := []string{
		"nav", "menu", "sidebar", "footer", "header",
		"cookie", "banner", "modal", "popup", "overlay",
		"breadcrumb", "pagination",
	}

	var extract func(*html.Node, int)
	extract = func(n *html.Node, depth int) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)

			// Skip certain tags
			if skipTags[tag] {
				return
			}

			// Skip elements with navigation/menu classes
			for _, attr := range n.Attr {
				if attr.Key == "class" || attr.Key == "role" || attr.Key == "aria-label" {
					val := strings.ToLower(attr.Val)
					for _, skip := range skipClasses {
						if strings.Contains(val, skip) {
							return
						}
					}
				}
			}

			// Extract heading text
			if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
				text := cleanText(getTextContent(n))
				if len(text) > 3 && len(text) < 200 {
					level := int(tag[1] - '0')
					prefix := strings.Repeat("#", level)
					sections = append(sections, fmt.Sprintf("%s %s", prefix, text))
					headings++
				}
				return // Don't recurse into heading children
			}

			// Extract link text
			if tag == "a" {
				href := getAttr(n, "href")
				text := cleanText(getTextContent(n))
				if len(text) > 3 && len(text) < 200 && href != "" && href != "#" {
					sections = append(sections, fmt.Sprintf("[%s](%s)", text, href))
					links++
				}
				return
			}

			// Extract paragraph text
			if tag == "p" || tag == "li" || tag == "td" || tag == "dd" {
				text := cleanText(getTextContent(n))
				if len(text) > 20 {
					sections = append(sections, text)
				}
				return
			}

			// Extract div/span text only at leaf level (no block children)
			if tag == "div" || tag == "span" || tag == "section" || tag == "main" {
				// Recurse into children
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					extract(c, depth+1)
				}
				return
			}
		}

		// Recurse into children for non-handled elements
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c, depth+1)
		}
	}

	// Find body element
	var body *html.Node
	var findBody func(*html.Node)
	findBody = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "body" {
			body = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findBody(c)
		}
	}
	findBody(doc)

	if body != nil {
		extract(body, 0)
	}

	// Deduplicate sections
	seen := make(map[string]bool)
	var unique []string
	for _, s := range sections {
		normalized := strings.TrimSpace(s)
		if !seen[normalized] && normalized != "" {
			seen[normalized] = true
			unique = append(unique, normalized)
		}
	}

	// Remove very short/noisy sections
	var filtered []string
	noisePattern := regexp.MustCompile(`^(©|copyright|all rights reserved|cookie|privacy|terms)`)
	for _, s := range unique {
		lower := strings.ToLower(s)
		if noisePattern.MatchString(lower) {
			continue
		}
		filtered = append(filtered, s)
	}

	content := strings.Join(filtered, "\n\n")

	return TextFallbackResult{
		Content:  content,
		Length:   len(content),
		Headings: headings,
		Links:    links,
	}
}
