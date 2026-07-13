package engine

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type TextFallbackResult struct {
	Content  string
	Length   int
	Headings int
	Links    int
}

func TextFallback(htmlStr string) TextFallbackResult {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return TextFallbackResult{}
	}

	var sections []string
	var headings int
	var links int

	skipTags := map[string]bool{
		"script": true, "style": true, "noscript": true,
		"nav": true, "footer": true, "header": true,
		"svg": true, "iframe": true, "form": true,
	}

	skipClasses := []string{
		"nav", "menu", "sidebar", "footer", "header",
		"cookie", "banner", "modal", "popup", "overlay",
		"breadcrumb", "pagination",
	}

	var extract func(*html.Node, int)
	extract = func(n *html.Node, depth int) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)

			if skipTags[tag] {
				return
			}

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

			if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
				text := cleanText(getTextContent(n))
				if len(text) > 3 && len(text) < 200 {
					level := int(tag[1] - '0')
					prefix := strings.Repeat("#", level)
					sections = append(sections, fmt.Sprintf("%s %s", prefix, text))
					headings++
				}
				return
			}

			if tag == "a" {
				href := getAttr(n, "href")
				text := cleanText(getTextContent(n))
				if len(text) > 3 && len(text) < 200 && href != "" && href != "#" {
					sections = append(sections, fmt.Sprintf("[%s](%s)", text, href))
					links++
				}
				return
			}

			if tag == "p" || tag == "li" || tag == "td" || tag == "dd" {
				text := cleanText(getTextContent(n))
				if len(text) > 20 {
					sections = append(sections, text)
				}
				return
			}

			if tag == "div" || tag == "span" || tag == "section" || tag == "main" {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					extract(c, depth+1)
				}
				return
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c, depth+1)
		}
	}

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

	seen := make(map[string]bool)
	var unique []string
	for _, s := range sections {
		normalized := strings.TrimSpace(s)
		if !seen[normalized] && normalized != "" {
			seen[normalized] = true
			unique = append(unique, normalized)
		}
	}

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
