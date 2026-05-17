package engine

import (
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// PreprocessHTML applies a host-agnostic preprocessing pass: replace twoslash
// buttons, strip common chrome (nav/aside/footer/sidebar/cookie banners), then
// scope to the most likely main-content container.
func PreprocessHTML(htmlStr string) string {
	return PreprocessHTMLWithURL(htmlStr, nil)
}

// PreprocessHTMLWithURL keeps the URL parameter for API compatibility but the
// preprocessing pipeline is now entirely host-agnostic.
func PreprocessHTMLWithURL(htmlStr string, _ *url.URL) string {
	htmlStr = replaceTwoslashButtons(htmlStr)
	htmlStr = stripCommonChrome(htmlStr)
	htmlStr = unwrapLayoutTables(htmlStr)
	htmlStr = stripCommentContainers(htmlStr)
	htmlStr = stripHighLinkDensityBlocks(htmlStr)
	htmlStr = scopeMainContent(htmlStr)
	return htmlStr
}

var twoslashButtonPattern = regexp.MustCompile(`<button\s+[^>]*class=["'][^"']*twoslash[^"']*["'][^>]*>(.*?)</button>`)

func replaceTwoslashButtons(htmlStr string) string {
	return twoslashButtonPattern.ReplaceAllString(htmlStr, "<span>$1</span>")
}

// chromeClassNeedles is matched case-insensitively against the class/id
// attribute string. Selectors are deliberately conservative to avoid eating
// real article content.
var chromeClassNeedles = []string{
	"sidebar",
	"navbox",
	"mw-editsection",
	"hatnote",
	"infobox-image",
	"cookie-banner",
	"breadcrumb",
	"breadcrumbs",
	"share-buttons",
	"social-share",
	"skip-link",
	"vp-footer",
}

var chromeAriaNeedles = []string{
	"cookie",
	"consent banner",
}

// stripCommonChrome removes well-known chrome elements from the HTML before
// the main-content anchor pass runs, so chrome can't dominate the
// largest-text-bearing-div fallback.
func stripCommonChrome(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}
	stripChromeNodes(doc)
	return renderNode(doc)
}

func stripChromeNodes(n *html.Node) {
	// Walk via an explicit list so we can mutate during iteration.
	var children []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		children = append(children, c)
	}
	for _, c := range children {
		if c.Type == html.ElementNode && shouldStripAsChrome(c) {
			n.RemoveChild(c)
			continue
		}
		stripChromeNodes(c)
	}
}

func shouldStripAsChrome(n *html.Node) bool {
	tag := n.DataAtom
	switch tag {
	case atom.Nav, atom.Aside, atom.Footer:
		return true
	case atom.Header:
		// Strip a <header> only if it contains a <nav> child — bare headers
		// frequently carry the article title.
		if hasDescendant(n, atom.Nav) {
			return true
		}
	}

	role := strings.ToLower(getAttr(n, "role"))
	switch role {
	case "banner", "navigation", "contentinfo":
		return true
	}

	classes := tokenize(strings.ToLower(getAttr(n, "class")))
	id := strings.ToLower(getAttr(n, "id"))
	for _, needle := range chromeClassNeedles {
		// Match exact class token, or exact id, to avoid eating containers
		// like "layout__2-sidebars-inline" or "page-content".
		if classes[needle] || id == needle {
			return true
		}
	}

	if aria := strings.ToLower(getAttr(n, "aria-label")); aria != "" {
		for _, needle := range chromeAriaNeedles {
			if strings.Contains(aria, needle) {
				return true
			}
		}
	}

	return false
}

func tokenize(s string) map[string]bool {
	out := map[string]bool{}
	for _, t := range strings.Fields(s) {
		if t != "" {
			out[t] = true
		}
	}
	return out
}

func hasDescendant(n *html.Node, a atom.Atom) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.DataAtom == a {
			return true
		}
		if hasDescendant(c, a) {
			return true
		}
	}
	return false
}

// scopeMainContent rewrites the document body to contain just the most likely
// main-content container, wrapped in <article> so go-readability locks onto it.
//
// Anchor priority: <main> → <article> → largest text-bearing <div>/<section>
// at depth ≥ 2 (≥ 200 chars of visible text).
//
// If no anchor is found, the input is returned unchanged.
func scopeMainContent(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}

	body := findFirstByAtom(doc, atom.Body)
	if body == nil {
		return htmlStr
	}

	anchor := findFirstByAtom(body, atom.Main)
	if anchor == nil {
		// Only anchor on <article> when there's exactly one — multiple
		// siblings (blog index, feed) means the real content shell is the
		// parent container, not any single article.
		articles := findAllByAtom(body, atom.Article)
		if len(articles) == 1 && visibleTextLen(articles[0]) >= 200 {
			anchor = articles[0]
		}
	}
	if anchor == nil {
		anchor = findLargestContainer(body)
	}
	if anchor == nil {
		return htmlStr
	}

	// Build a synthetic <article> that wraps the anchor's children.
	article := &html.Node{Type: html.ElementNode, Data: "article", DataAtom: atom.Article}
	for c := anchor.FirstChild; c != nil; {
		next := c.NextSibling
		anchor.RemoveChild(c)
		article.AppendChild(c)
		c = next
	}

	// Clear the body and re-attach only the wrapped anchor content.
	for c := body.FirstChild; c != nil; {
		next := c.NextSibling
		body.RemoveChild(c)
		c = next
	}
	body.AppendChild(article)

	return renderNode(doc)
}

func findAllByAtom(root *html.Node, a atom.Atom) []*html.Node {
	var out []*html.Node
	var visit func(n *html.Node)
	visit = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode && n.DataAtom == a {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	return out
}

func findFirstByAtom(root *html.Node, a atom.Atom) *html.Node {
	if root == nil {
		return nil
	}
	if root.Type == html.ElementNode && root.DataAtom == a {
		return root
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if got := findFirstByAtom(c, a); got != nil {
			return got
		}
	}
	return nil
}

// findLargestContainer picks the <div> or <section> at depth ≥ 2 (relative to
// body) with the highest visible-text length. Returns nil if no candidate has
// at least 200 chars of text.
func findLargestContainer(body *html.Node) *html.Node {
	const minTextLen = 200

	var best *html.Node
	var bestLen int

	var visit func(n *html.Node, depth int)
	visit = func(n *html.Node, depth int) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode &&
			(n.DataAtom == atom.Div || n.DataAtom == atom.Section) &&
			depth >= 2 {
			length := visibleTextLen(n)
			if length > bestLen && length >= minTextLen {
				best = n
				bestLen = length
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c, depth+1)
		}
	}
	visit(body, 0)
	return best
}

// containsContentShell reports whether n contains a <main>, an <article>, or
// a prose <p> (a paragraph with ≥ 50 chars of non-link text). Used by
// stripHighLinkDensityBlocks to avoid nuking real article shells that happen
// to wrap link-dense reference lists.
func containsContentShell(n *html.Node) bool {
	if n == nil {
		return false
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			switch c.DataAtom {
			case atom.Main, atom.Article:
				return true
			case atom.P:
				total := visibleTextLen(c)
				link := linkTextLen(c)
				if total-link >= 50 {
					return true
				}
			}
		}
		if containsContentShell(c) {
			return true
		}
	}
	return false
}

// linkTextLen sums the visible-text length contained inside <a> descendants
// of n. Used by stripHighLinkDensityBlocks to estimate link-to-text ratio.
func linkTextLen(n *html.Node) int {
	if n == nil {
		return 0
	}
	total := 0
	for _, a := range findAllByAtom(n, atom.A) {
		total += visibleTextLen(a)
	}
	return total
}

// stripHighLinkDensityBlocks removes container blocks whose visible text is
// dominated by anchor text — tag clouds, related-article widgets, footer
// link-grids that the selector-based stripCommonChrome pass missed. Pure
// structural heuristic, no host-specific knowledge.
//
// A container (div/ul/ol/aside at depth ≥ 2 from body) is removed when:
//   - total visible text ≥ 50 chars, AND
//   - it contains ≥ 3 anchors, AND
//   - linkText / totalText > 0.7
//
// <section> is deliberately excluded from the candidate set: it is a semantic
// content-division element, almost never structural boilerplate, and including
// it nukes legitimate reference-style sections (e.g. MDN's "See also" or
// "Specifications" link tables). The widgets we want to catch — tag clouds,
// related-articles widgets, footer link grids — are virtually always div/ul/aside.
func stripHighLinkDensityBlocks(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}
	body := findFirstByAtom(doc, atom.Body)
	if body == nil {
		return htmlStr
	}

	const (
		minTotalText = 50
		minAnchors   = 3
		ratioCutoff  = 0.7
	)

	bodyTotal := visibleTextLen(body)

	var marked []*html.Node
	var visit func(n *html.Node, depth int)
	visit = func(n *html.Node, depth int) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode && depth >= 2 {
			switch n.DataAtom {
			case atom.Div, atom.Ul, atom.Ol, atom.Aside:
				total := visibleTextLen(n)
				if total >= minTotalText {
					anchors := findAllByAtom(n, atom.A)
					if len(anchors) >= minAnchors {
						link := linkTextLen(n)
						if total > 0 && float64(link)/float64(total) > ratioCutoff {
							// Belt + braces: a container that holds the
							// page's <main> or a lone <article> is the
							// content shell, not boilerplate — skip it
							// regardless of link density and let the
							// scopeMainContent pass anchor inside it.
							if containsContentShell(n) {
								// fall through to descend
							} else if bodyTotal-total < 200 {
								// The candidate is essentially the only
								// substantive content on the page —
								// removing it would empty the body (this
								// is what link-only "text-only" homepages
								// and article-index pages look like).
								// Let the index-fallback handle it.
								// fall through to descend
							} else {
								marked = append(marked, n)
								// Don't descend into a node we plan to remove.
								return
							}
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c, depth+1)
		}
	}
	visit(body, 0)

	if len(marked) == 0 {
		return htmlStr
	}
	for _, n := range marked {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}
	return renderNode(doc)
}

// visibleTextLen sums the length of text-node content under n, skipping
// <script>, <style>, and <noscript> subtrees.
func visibleTextLen(n *html.Node) int {
	if n == nil {
		return 0
	}
	if n.Type == html.ElementNode {
		switch n.DataAtom {
		case atom.Script, atom.Style, atom.Noscript:
			return 0
		}
	}
	if n.Type == html.TextNode {
		return len(strings.TrimSpace(n.Data))
	}
	total := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		total += visibleTextLen(c)
	}
	return total
}

func renderNode(n *html.Node) string {
	var b strings.Builder
	if err := html.Render(&b, n); err != nil {
		return ""
	}
	return b.String()
}
