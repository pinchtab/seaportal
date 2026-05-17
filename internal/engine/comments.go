package engine

import (
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// CommentRef is a single user-generated comment harvested from a recognised
// comment-container element (Disqus, native comments, "Replies" widgets).
// Surfaced on Result.Comments when the caller opts in via Options.WithComments.
type CommentRef struct {
	Author    string `json:"author,omitempty"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp,omitempty"`
}

// commentContainerTokens is the set of id/class tokens that mark a
// user-comments section. Match is exact-token (whitespace + dash + dot split),
// lowercased.
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

// detectCommentContainer reports whether n is a user-comments container based
// purely on its attributes (id/class/role/data-*). Never inspects text — a
// page about "JS comments" must not trip this.
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

// tokenizeCommentAttr splits a class/id attribute on whitespace, dashes, and
// dots so we can match against composite tokens like "comment-list" as well as
// the bare ones inside them.
func tokenizeCommentAttr(s string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range strings.Fields(s) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		out[raw] = true
		// Also expose composite tokens split by '-' and '.' so single-word
		// tokens like "comments" inside class="post-comments" can match.
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

// stripCommentContainers removes detected comment-container subtrees from the
// DOM and returns the modified HTML. Always-on in preprocess — comments are
// infrastructure noise we never want in main content.
//
// Body-emptiness guard: if removing all candidates would leave <body> with
// less than 200 chars of visible text, the strip is aborted and the original
// input returned unchanged. This guards the rare case where a comment widget
// wraps the whole article.
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

	// Walk, collecting candidates at depth ≥ 2 from body (so we never nuke
	// the page root or a top-level wrapper that happens to be classed
	// "comments"). Don't descend into a candidate once chosen — nested
	// comment markup is removed wholesale with its container.
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

	// Body-emptiness guard: estimate what visible text would remain after
	// removal. Sum candidate text and subtract from body total.
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

// ExtractComments walks htmlStr for comment containers and harvests one
// CommentRef per detected comment block, best-effort. baseURL is reserved for
// future use (resolving relative author profile links). Returns nil when no
// containers are found.
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
			// Treat the whole container as one comment.
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

// findCommentBlocks returns child sub-blocks within a comment container that
// look like individual comments — repeating li/article/div elements whose
// class hints at being a single comment. Best-effort.
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
	// itemtype microdata is a clean structural signal.
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

// findCommentText picks the longest text-bearing child as the comment body,
// preferring an explicit .comment-body / .comment-text / [itemprop=text] child
// when present. Falls back to the container's own visible text.
func findCommentText(n *xhtml.Node) string {
	// Preferred explicit body.
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

// textContent walks descendant text nodes, joining with single spaces; skips
// script/style.
func textContent(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(c *xhtml.Node)
	walk = func(c *xhtml.Node) {
		if c == nil {
			return
		}
		if c.Type == xhtml.ElementNode {
			switch c.DataAtom {
			case atom.Script, atom.Style, atom.Noscript:
				return
			}
		}
		if c.Type == xhtml.TextNode {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(c.Data)
		}
		for k := c.FirstChild; k != nil; k = k.NextSibling {
			walk(k)
		}
	}
	walk(n)
	return b.String()
}
