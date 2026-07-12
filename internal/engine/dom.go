package engine

// dom.go — shared DOM/string primitives for the engine package: attribute
// lookup, descendant-text collection, and whitespace collapsing. These were
// previously duplicated across index_extract.go, snapshot.go, schema.go,
// comments.go, tables.go, and links.go.

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// getAttr returns the value of the first attribute named key on n, or ""
// when absent. Key match is exact (x/net/html lowercases attribute keys at
// parse time).
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// hasAttr reports whether n carries an attribute named key (boolean twin of
// getAttr — present-but-empty attributes count).
func hasAttr(n *html.Node, key string) bool {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return true
		}
	}
	return false
}

// textOptions parameterizes nodeTextOpts. The zero value concatenates every
// descendant text node with no separator and no subtree skipping.
type textOptions struct {
	// spaceJoin inserts a single space between consecutive text nodes
	// (before each text node once the buffer is non-empty).
	spaceJoin bool
	// skipScriptStyle skips <script>/<style> subtrees.
	skipScriptStyle bool
	// skipNoscript additionally skips <noscript> subtrees.
	skipNoscript bool
}

// nodeTextOpts walks n's subtree in document order and collects descendant
// text-node data per opts. Callers keep their own trimming/collapsing so
// each call site's output is unchanged.
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

// nodeText concatenates the descendant text nodes of n with no separator.
// When skipScriptStyle is true, <script>/<style> subtrees are skipped.
func nodeText(n *html.Node, skipScriptStyle bool) string {
	return nodeTextOpts(n, textOptions{skipScriptStyle: skipScriptStyle})
}

// getTextContent is the historical no-skip concatenating walker used by the
// index-page and text-fallback extractors.
func getTextContent(n *html.Node) string {
	return nodeText(n, false)
}

// wsRunRE matches runs of regexp whitespace ([\t\n\f\r ] — note: NOT \v and
// NOT Unicode spaces). Shared by cleanText, dedupe's normalizeBlock, and the
// fingerprint normalizer so the pattern is compiled exactly once.
var wsRunRE = regexp.MustCompile(`\s+`)

// cleanText collapses regexp-whitespace runs to single spaces, then trims
// Unicode whitespace from both ends.
//
// NOT interchangeable with collapseWhitespace: the trailing strings.TrimSpace
// also trims Unicode spaces (NBSP, U+0085, …) from the edges, and interior
// \v is left alone (Go's regexp \s excludes \v).
func cleanText(s string) string {
	return strings.TrimSpace(wsRunRE.ReplaceAllString(s, " "))
}

// collapseWhitespace trims leading/trailing whitespace and collapses internal
// whitespace runs (any mix of spaces/tabs/newlines) into a single space.
// Allocation-free single pass; ASCII whitespace only — Unicode spaces such as
// NBSP are preserved verbatim (deliberate: &nbsp; is content, not layout).
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

// collapseUnicodeWhitespace splits on Unicode whitespace (including NBSP) and
// re-joins with single spaces, trimming the ends as a side effect. Distinct
// from collapseWhitespace, which is ASCII-only and NBSP-preserving.
func collapseUnicodeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
