package engine

import (
	"fmt"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func applySelectorOps(htmlStr, selectCSS, stripCSS string) (string, []string) {
	if selectCSS == "" && stripCSS == "" {
		return htmlStr, nil
	}

	var warnings []string

	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("selector-ops parse failed: %v", err))
		return htmlStr, warnings
	}

	if stripCSS != "" {
		for _, raw := range splitSelectors(stripCSS) {
			sel, compileErr := cascadia.Compile(raw)
			if compileErr != nil {
				warnings = append(warnings, fmt.Sprintf("invalid --strip selector %q: %v", raw, compileErr))
				continue
			}
			matches := cascadia.QueryAll(doc, sel)
			for _, n := range matches {
				if n.Parent != nil {
					n.Parent.RemoveChild(n)
				}
			}
		}
	}

	if selectCSS != "" {
		var collected []*html.Node
		seen := map[*html.Node]bool{}
		for _, raw := range splitSelectors(selectCSS) {
			sel, compileErr := cascadia.Compile(raw)
			if compileErr != nil {
				warnings = append(warnings, fmt.Sprintf("invalid --select selector %q: %v", raw, compileErr))
				continue
			}
			for _, n := range cascadia.QueryAll(doc, sel) {
				if seen[n] {
					continue
				}
				seen[n] = true
				collected = append(collected, n)
			}
		}

		if len(collected) == 0 {
			warnings = append(warnings, fmt.Sprintf("no match for --select %q", selectCSS))
			return htmlStr, warnings
		}

		body := findFirstByAtom(doc, atom.Body)
		if body == nil {
			warnings = append(warnings, "no <body> element to rewrite for --select")
			return htmlStr, warnings
		}

		wrapper := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
		for _, n := range collected {
			if n.Parent != nil {
				n.Parent.RemoveChild(n)
			}
			wrapper.AppendChild(n)
		}

		for c := body.FirstChild; c != nil; {
			next := c.NextSibling
			body.RemoveChild(c)
			c = next
		}
		body.AppendChild(wrapper)
	} else if stripCSS != "" {
		if body := findFirstByAtom(doc, atom.Body); body != nil && !hasElementChild(body) {
			warnings = append(warnings, "strip removed substantial content")
		}
	}

	var buf strings.Builder
	if err := html.Render(&buf, doc); err != nil {
		warnings = append(warnings, fmt.Sprintf("selector-ops render failed: %v", err))
		return htmlStr, warnings
	}
	return buf.String(), warnings
}

func splitSelectors(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func hasElementChild(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return true
		}
	}
	return false
}
