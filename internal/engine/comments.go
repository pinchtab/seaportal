package engine

import (
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type CommentRef struct {
	Author    string `json:"author,omitempty"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp,omitempty"`
}

var commentContainerTokens = map[string]bool{
	"comments":        true,
	"comment-list":    true,
	"disqus_thread":   true,
	"disqus":          true,
	"respond":         true,
	"replies":         true,
	"comment-area":    true,
	"commentwrap":     true,
	"comment-section": true,
}

func detectCommentContainer(n *xhtml.Node) bool {
	if n == nil || n.Type != xhtml.ElementNode {
		return false
	}

	id := strings.ToLower(strings.TrimSpace(getAttr(n, "id")))
	if id != "" && commentContainerTokens[id] {
		return true
	}

	classes := tokenizeCommentAttr(strings.ToLower(getAttr(n, "class")))
	for tok := range classes {
		if commentContainerTokens[tok] {
			return true
		}
	}

	if strings.EqualFold(getAttr(n, "data-component"), "comments") {
		return true
	}
	if strings.EqualFold(getAttr(n, "data-element"), "comments") {
		return true
	}

	if strings.EqualFold(getAttr(n, "role"), "region") {
		aria := strings.ToLower(getAttr(n, "aria-label"))
		if aria != "" && strings.Contains(aria, "comment") {
			return true
		}
	}

	return false
}

func tokenizeCommentAttr(s string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range strings.Fields(s) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		out[raw] = true
		for _, part := range splitMany(raw, "-.") {
			if part != "" {
				out[part] = true
			}
		}
	}
	return out
}

func splitMany(s, seps string) []string {
	f := func(r rune) bool { return strings.ContainsRune(seps, r) }
	return strings.FieldsFunc(s, f)
}

func stripCommentContainers(htmlStr string) string {
	if htmlStr == "" {
		return htmlStr
	}
	doc, err := xhtml.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}
	body := findFirstByAtom(doc, atom.Body)
	if body == nil {
		return htmlStr
	}

	var marked []*xhtml.Node
	var visit func(n *xhtml.Node, depth int)
	visit = func(n *xhtml.Node, depth int) {
		if n == nil {
			return
		}
		if n.Type == xhtml.ElementNode && depth >= 2 && detectCommentContainer(n) {
			marked = append(marked, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c, depth+1)
		}
	}
	visit(body, 0)

	if len(marked) == 0 {
		return htmlStr
	}

	bodyTotal := visibleTextLen(body)
	var removedText int
	for _, n := range marked {
		removedText += visibleTextLen(n)
	}
	if bodyTotal-removedText < 200 {
		return htmlStr
	}

	for _, n := range marked {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}
	return renderNode(doc)
}

func ExtractComments(htmlStr string, _ string) []CommentRef {
	if htmlStr == "" {
		return nil
	}
	doc, err := xhtml.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}
	body := findFirstByAtom(doc, atom.Body)
	if body == nil {
		return nil
	}

	var containers []*xhtml.Node
	var visit func(n *xhtml.Node, depth int)
	visit = func(n *xhtml.Node, depth int) {
		if n == nil {
			return
		}
		if n.Type == xhtml.ElementNode && depth >= 2 && detectCommentContainer(n) {
			containers = append(containers, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c, depth+1)
		}
	}
	visit(body, 0)

	if len(containers) == 0 {
		return nil
	}

	var out []CommentRef
	for _, c := range containers {
		blocks := findCommentBlocks(c)
		if len(blocks) == 0 {
			ref := buildCommentRef(c)
			if ref.Text != "" {
				out = append(out, ref)
			}
			continue
		}
		for _, b := range blocks {
			ref := buildCommentRef(b)
			if ref.Text != "" {
				out = append(out, ref)
			}
		}
	}
	return out
}

func findCommentBlocks(container *xhtml.Node) []*xhtml.Node {
	var out []*xhtml.Node
	var visit func(n *xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n == nil {
			return
		}
		if n != container && n.Type == xhtml.ElementNode {
			switch n.DataAtom {
			case atom.Li, atom.Article, atom.Div:
				if looksLikeCommentBlock(n) {
					out = append(out, n)
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(container)
	return out
}

func looksLikeCommentBlock(n *xhtml.Node) bool {
	classes := tokenizeCommentAttr(strings.ToLower(getAttr(n, "class")))
	id := strings.ToLower(getAttr(n, "id"))
	if itype := getAttr(n, "itemtype"); itype != "" {
		if strings.Contains(strings.ToLower(itype), "comment") {
			return true
		}
	}
	for tok := range classes {
		if tok == "comment" || tok == "reply" || strings.HasPrefix(tok, "comment-") {
			return true
		}
	}
	if strings.HasPrefix(id, "comment-") || id == "comment" {
		return true
	}
	return false
}

func buildCommentRef(n *xhtml.Node) CommentRef {
	ref := CommentRef{}
	ref.Author = findAuthor(n)
	ref.Timestamp = findTimestamp(n)
	ref.Text = findCommentText(n)
	return ref
}

func findAuthor(n *xhtml.Node) string {
	var found string
	var visit func(c *xhtml.Node)
	visit = func(c *xhtml.Node) {
		if c == nil || found != "" {
			return
		}
		if c.Type == xhtml.ElementNode {
			if v := getAttr(c, "itemprop"); strings.EqualFold(v, "author") {
				found = strings.TrimSpace(html.UnescapeString(textContent(c)))
				return
			}
			if v := getAttr(c, "data-author"); v != "" {
				found = strings.TrimSpace(html.UnescapeString(v))
				return
			}
			classes := tokenizeCommentAttr(strings.ToLower(getAttr(c, "class")))
			if classes["author"] || classes["comment-author"] || classes["username"] {
				found = strings.TrimSpace(html.UnescapeString(textContent(c)))
				return
			}
		}
		for k := c.FirstChild; k != nil; k = k.NextSibling {
			visit(k)
		}
	}
	visit(n)
	return collapseWhitespace(found)
}

func findTimestamp(n *xhtml.Node) string {
	var found string
	var visit func(c *xhtml.Node)
	visit = func(c *xhtml.Node) {
		if c == nil || found != "" {
			return
		}
		if c.Type == xhtml.ElementNode {
			if c.DataAtom == atom.Time {
				if v := getAttr(c, "datetime"); v != "" {
					found = strings.TrimSpace(v)
					return
				}
				found = strings.TrimSpace(textContent(c))
				if found != "" {
					return
				}
			}
			if v := getAttr(c, "data-time"); v != "" {
				found = strings.TrimSpace(v)
				return
			}
			classes := tokenizeCommentAttr(strings.ToLower(getAttr(c, "class")))
			if classes["timestamp"] || classes["comment-time"] || classes["date"] {
				found = strings.TrimSpace(textContent(c))
				if found != "" {
					return
				}
			}
		}
		for k := c.FirstChild; k != nil; k = k.NextSibling {
			visit(k)
		}
	}
	visit(n)
	return collapseWhitespace(found)
}

func findCommentText(n *xhtml.Node) string {
	var preferred *xhtml.Node
	var visit func(c *xhtml.Node)
	visit = func(c *xhtml.Node) {
		if c == nil || preferred != nil {
			return
		}
		if c.Type == xhtml.ElementNode {
			if v := getAttr(c, "itemprop"); strings.EqualFold(v, "text") || strings.EqualFold(v, "commentText") {
				preferred = c
				return
			}
			classes := tokenizeCommentAttr(strings.ToLower(getAttr(c, "class")))
			if classes["comment-body"] || classes["comment-text"] || classes["comment-content"] {
				preferred = c
				return
			}
		}
		for k := c.FirstChild; k != nil; k = k.NextSibling {
			visit(k)
		}
	}
	visit(n)

	var raw string
	if preferred != nil {
		raw = textContent(preferred)
	} else {
		raw = textContent(n)
	}
	return collapseWhitespace(strings.TrimSpace(html.UnescapeString(raw)))
}

func textContent(n *xhtml.Node) string {
	return nodeTextOpts(n, textOptions{spaceJoin: true, skipScriptStyle: true, skipNoscript: true})
}
