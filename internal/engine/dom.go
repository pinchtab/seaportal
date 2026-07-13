package engine

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return true
		}
	}
	return false
}

type textOptions struct {
	spaceJoin       bool
	skipScriptStyle bool
	skipNoscript    bool
}

func nodeTextOpts(n *html.Node, opts textOptions) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if node.Type == html.ElementNode {
			switch node.DataAtom {
			case atom.Script, atom.Style:
				if opts.skipScriptStyle {
					return
				}
			case atom.Noscript:
				if opts.skipNoscript {
					return
				}
			}
		}
		if node.Type == html.TextNode {
			if opts.spaceJoin && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func nodeText(n *html.Node, skipScriptStyle bool) string {
	return nodeTextOpts(n, textOptions{skipScriptStyle: skipScriptStyle})
}

func getTextContent(n *html.Node) string {
	return nodeText(n, false)
}

var wsRunRE = regexp.MustCompile(`\s+`)

func cleanText(s string) string {
	return strings.TrimSpace(wsRunRE.ReplaceAllString(s, " "))
}

func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	started := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v' {
			if started {
				inSpace = true
			}
			continue
		}
		if inSpace {
			b.WriteByte(' ')
			inSpace = false
		}
		b.WriteRune(r)
		started = true
	}
	return b.String()
}

func collapseUnicodeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
