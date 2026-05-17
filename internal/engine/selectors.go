package engine

import (
	"fmt"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// applySelectorOps applies optional CSS-selector targeting to htmlStr:
//
//  1. If stripCSS is non-empty, every node matching one of the comma-separated
//     selectors is removed from its parent. Strip always runs FIRST.
//  2. If selectCSS is non-empty, the document body is rewritten to contain
//     only the subtrees matching one of the comma-separated selectors. The
//     original <head> is preserved so downstream charset/meta extraction
//     continues to work. Multiple matches are concatenated under a single
//     <div> wrapper so downstream extraction sees one subtree.
//
// On any internal failure the original input is returned unchanged together
// with a warning explaining what happened. Invalid selectors are skipped
// individually with a per-selector warning; the rest of the operation
// continues. A --select that matches zero nodes warns and returns the input
// untouched (we never want to wipe the page silently).
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

	// --strip runs first so --select sees the post-strip DOM.
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

		// Wrap matched subtrees in a single <div>.
		wrapper := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
		for _, n := range collected {
			if n.Parent != nil {
				n.Parent.RemoveChild(n)
			}
			wrapper.AppendChild(n)
		}

		// Clear body and re-attach the wrapper. <head> is untouched.
		for c := body.FirstChild; c != nil; {
			next := c.NextSibling
			body.RemoveChild(c)
			c = next
		}
		body.AppendChild(wrapper)
	} else if stripCSS != "" {
		// Strip-only path: warn if the body is now empty of element children.
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
