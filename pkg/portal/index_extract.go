// Package portal provides content extraction with SPA detection
package portal

import (
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

type IndexPageResult struct {
	IsIndexPage   bool
	Items         []CardItem
	Markdown      string
	Confidence    int
	ArticleCount  int
	HeadlineCount int
}

type CardItem struct {
	Title   string
	URL     string
	Teaser  string
	Section string
}

// DetectIndexPage checks if HTML looks like a homepage/index page
// (multiple article cards rather than single article content)
func DetectIndexPage(htmlStr string) IndexPageResult {
	result := IndexPageResult{}

	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return result
	}

	articleCount := countElements(doc, "article")
	// Extended card class patterns - includes modern React/styled-component patterns
	cardCount := countElementsWithClass(doc, "div", []string{
		"card", "post", "story", "item", "entry", "teaser",
		"cardwrapper", "summaryitem", "headline", "hed",
	})

	result.ArticleCount = articleCount + cardCount

	items := extractCardItems(doc)
	result.Items = items
	result.HeadlineCount = len(items)

	// Determine if this is an index page
	// Heuristic: 5+ article/card elements with headlines suggests index page
	if result.ArticleCount >= 5 && result.HeadlineCount >= 5 { //nolint:gocritic
		result.IsIndexPage = true
		result.Confidence = min(100, 50+result.HeadlineCount*2)
	} else if result.HeadlineCount >= 8 {
		// Lots of headlines even without article elements - lowered threshold
		result.IsIndexPage = true
		result.Confidence = min(100, 40+result.HeadlineCount*2)
	} else if result.HeadlineCount >= 6 && result.ArticleCount >= 3 {
		// Mixed case: some cards + decent headlines
		result.IsIndexPage = true
		result.Confidence = min(100, 35+result.HeadlineCount*2)
	}

	if result.IsIndexPage && len(items) > 0 {
		result.Markdown = formatIndexMarkdown(items)
	}

	return result
}

func ShouldUseIndexFallback(readabilityResult Result, indexResult IndexPageResult) bool {
	if !indexResult.IsIndexPage {
		return false
	}

	// Use fallback if readability gave poor results on an index page
	// Poor results = low content length or few headings despite many articles
	// Raised thresholds: homepages should have substantial content
	readabilityPoor := readabilityResult.Length < 4000 ||
		readabilityResult.HeadingCount < 5

	indexRich := indexResult.HeadlineCount >= 5

	// Also use fallback if readability found almost no headings but index has many
	// (suggests readability completely missed the article list)
	readabilityMissedContent := readabilityResult.HeadingCount < 4 &&
		indexResult.HeadlineCount >= 10

	return (readabilityPoor && indexRich) || readabilityMissedContent
}

func countElements(n *html.Node, tag string) int {
	count := 0
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			count++
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return count
}

func countElementsWithClass(n *html.Node, tag string, classPatterns []string) int {
	count := 0
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			class := getAttr(n, "class")
			classLower := strings.ToLower(class)
			for _, pattern := range classPatterns {
				if strings.Contains(classLower, pattern) {
					count++
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return count
}

func extractCardItems(doc *html.Node) []CardItem {
	var items []CardItem
	seen := make(map[string]bool)

	// Strategy 1: Find headlines inside <article> elements
	articles := findElements(doc, "article")
	for _, article := range articles {
		item := extractCardFromArticle(article)
		if item.Title != "" && item.URL != "" && !seen[item.URL] {
			items = append(items, item)
			seen[item.URL] = true
		}
	}

	// Strategy 2: Find h2/h3 elements with links (common pattern)
	for _, tag := range []string{"h2", "h3", "h4"} {
		headings := findElements(doc, tag)
		for _, h := range headings {
			item := extractCardFromHeading(h)
			if item.Title != "" && item.URL != "" && !seen[item.URL] {
				items = append(items, item)
				seen[item.URL] = true
			}
		}
	}

	// Sort by title length (longer titles usually more informative)
	sort.Slice(items, func(i, j int) bool {
		return len(items[i].Title) > len(items[j].Title)
	})

	if len(items) > 50 {
		items = items[:50]
	}

	return items
}

func extractCardFromArticle(article *html.Node) CardItem {
	item := CardItem{}

	for _, tag := range []string{"h1", "h2", "h3", "h4"} {
		if h := findFirstElement(article, tag); h != nil {
			item.Title = cleanText(getTextContent(h))
			if a := findFirstElement(h, "a"); a != nil {
				item.URL = getAttr(a, "href")
			}
			break
		}
	}

	if item.URL == "" {
		if a := findFirstElement(article, "a"); a != nil {
			item.URL = getAttr(a, "href")
			if item.Title == "" {
				item.Title = cleanText(getTextContent(a))
			}
		}
	}

	teaserClasses := []string{"excerpt", "teaser", "summary", "description", "deck", "dek"}
	for _, class := range teaserClasses {
		if el := findElementWithClass(article, class); el != nil {
			item.Teaser = cleanText(getTextContent(el))
			break
		}
	}

	// Fallback: first <p> that's not too long
	if item.Teaser == "" {
		if p := findFirstElement(article, "p"); p != nil {
			text := cleanText(getTextContent(p))
			if len(text) > 20 && len(text) < 500 {
				item.Teaser = text
			}
		}
	}

	return item
}

func extractCardFromHeading(h *html.Node) CardItem {
	item := CardItem{}

	if a := findFirstElement(h, "a"); a != nil {
		item.Title = cleanText(getTextContent(a))
		item.URL = getAttr(a, "href")
	} else {
		item.Title = cleanText(getTextContent(h))
		if parent := h.Parent; parent != nil {
			if a := findFirstElement(parent, "a"); a != nil {
				item.URL = getAttr(a, "href")
			}
		}
	}

	item.Teaser = findTeaserAfterHeading(h)

	return item
}

func findTeaserAfterHeading(h *html.Node) string {
	for sib := h.NextSibling; sib != nil; sib = sib.NextSibling {
		if sib.Type == html.ElementNode {
			// Stop at next heading - we've gone too far
			if sib.Data == "h1" || sib.Data == "h2" || sib.Data == "h3" || sib.Data == "h4" {
				break
			}
			if sib.Data == "p" {
				text := cleanText(getTextContent(sib))
				if len(text) > 30 && len(text) < 500 {
					return text
				}
			}
			class := strings.ToLower(getAttr(sib, "class"))
			for _, pattern := range []string{"excerpt", "teaser", "summary", "description", "deck", "dek", "blurb", "lead"} {
				if strings.Contains(class, pattern) {
					text := cleanText(getTextContent(sib))
					if len(text) > 20 && len(text) < 500 {
						return text
					}
				}
			}
		}
	}

	if parent := h.Parent; parent != nil {
		for c := parent.FirstChild; c != nil; c = c.NextSibling {
			if c == h {
				continue // Skip the heading itself
			}
			if c.Type == html.ElementNode {
				class := strings.ToLower(getAttr(c, "class"))
				for _, pattern := range []string{"excerpt", "teaser", "summary", "description", "deck", "dek", "blurb", "lead"} {
					if strings.Contains(class, pattern) {
						text := cleanText(getTextContent(c))
						if len(text) > 20 && len(text) < 500 {
							return text
						}
					}
				}
				if c.Data == "p" {
					text := cleanText(getTextContent(c))
					if len(text) > 30 && len(text) < 500 {
						return text
					}
				}
			}
		}
	}

	return ""
}

func findElements(n *html.Node, tag string) []*html.Node {
	var result []*html.Node
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			result = append(result, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return result
}

func findFirstElement(n *html.Node, tag string) *html.Node {
	var result *html.Node
	var f func(*html.Node) bool
	f = func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == tag {
			result = n
			return true
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if f(c) {
				return true
			}
		}
		return false
	}
	f(n)
	return result
}

func findElementWithClass(n *html.Node, classPattern string) *html.Node {
	var result *html.Node
	var f func(*html.Node) bool
	f = func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			class := strings.ToLower(getAttr(n, "class"))
			if strings.Contains(class, classPattern) {
				result = n
				return true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if f(c) {
				return true
			}
		}
		return false
	}
	f(n)
	return result
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func getTextContent(n *html.Node) string {
	var sb strings.Builder
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return sb.String()
}

func cleanText(s string) string {
	re := regexp.MustCompile(`\s+`)
	s = re.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func formatIndexMarkdown(items []CardItem) string {
	var sb strings.Builder

	for _, item := range items {
		if item.Title == "" {
			continue
		}

		// Format: ## [Title](URL)
		if item.URL != "" {
			sb.WriteString("## [")
			sb.WriteString(item.Title)
			sb.WriteString("](")
			sb.WriteString(item.URL)
			sb.WriteString(")\n")
		} else {
			sb.WriteString("## ")
			sb.WriteString(item.Title)
			sb.WriteString("\n")
		}

		if item.Teaser != "" {
			sb.WriteString("\n")
			sb.WriteString(item.Teaser)
			sb.WriteString("\n")
		}

		sb.WriteString("\n")
	}

	return sb.String()
}
