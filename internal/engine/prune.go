package engine

import (
	"math"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func PruneToContent(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}

	body := findFirstByAtom(doc, atom.Body)
	if body == nil {
		return htmlStr
	}

	indexByNode := map[*html.Node]int{}
	total := 0
	{
		var walk func(n *html.Node)
		walk = func(n *html.Node) {
			if n == nil {
				return
			}
			if n.Type == html.ElementNode {
				indexByNode[n] = total
				total++
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(body)
	}
	if total == 0 {
		return htmlStr
	}

	const minTextLen = 200

	var (
		best      *html.Node
		bestScore = math.Inf(-1)
		bestText  int
	)

	var visit func(n *html.Node)
	visit = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode && isPruneCandidate(n) {
			textLen := visibleTextLen(n)
			if textLen >= minTextLen {
				linkLen := linkTextLen(n)
				chromeChildren := countChromeChildren(n)
				base := float64(textLen)/float64(1+linkLen) - 100.0*float64(chromeChildren)
				idx := indexByNode[n]
				mid := float64(total) / 2.0
				var posBias float64
				if mid > 0 {
					posBias = 1.0 - math.Abs(float64(idx)-mid)/mid
					if posBias < 0 {
						posBias = 0
					}
				} else {
					posBias = 1.0
				}
				score := base * posBias
				if score > bestScore {
					bestScore = score
					best = n
					bestText = textLen
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(body)

	if best == nil || bestText < minTextLen {
		return htmlStr
	}

	article := &html.Node{Type: html.ElementNode, Data: "article", DataAtom: atom.Article}
	for c := best.FirstChild; c != nil; {
		next := c.NextSibling
		best.RemoveChild(c)
		article.AppendChild(c)
		c = next
	}
	for c := body.FirstChild; c != nil; {
		next := c.NextSibling
		body.RemoveChild(c)
		c = next
	}
	body.AppendChild(article)

	return renderNode(doc)
}

func isPruneCandidate(n *html.Node) bool {
	switch n.DataAtom {
	case atom.Div, atom.Section, atom.Article, atom.Main:
		return true
	}
	return false
}

func countChromeChildren(n *html.Node) int {
	count := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		switch c.DataAtom {
		case atom.Nav, atom.Aside, atom.Header, atom.Footer:
			count++
			continue
		}
		classes := tokenize(strings.ToLower(getAttr(c, "class")))
		id := strings.ToLower(getAttr(c, "id"))
		hit := false
		for _, needle := range chromeClassNeedles {
			if classes[needle] || id == needle {
				hit = true
				break
			}
		}
		if hit {
			count++
		}
	}
	return count
}
