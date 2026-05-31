package engine

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// LinkRef is a structured representation of a discovered <a> link in the raw
// page HTML. Surfaced on Result.Links when the caller opts in via
// Options.WithLinks so agents can pick the next page without re-parsing the
// extracted Markdown.
type LinkRef struct {
	Href string `json:"href"`
	Text string `json:"text,omitempty"`
	Rel  string `json:"rel,omitempty"`
}

const linkTextMaxLen = 200

// skippedSchemes drops non-navigational schemes: in-page handlers, mail
// helpers, phone dialers, embedded payloads.
var skippedSchemes = map[string]bool{
	"javascript": true,
	"mailto":     true,
	"tel":        true,
	"data":       true,
}

// ExtractLinks walks htmlStr and returns every <a href="…"> as a LinkRef in
// document order. Relative hrefs are resolved against baseURL; entries are
// deduplicated by the (href, text) pair (first occurrence wins). Anchors
// inside <script>/<style> subtrees, fragment-only hrefs (#…), empty hrefs,
// and javascript/mailto/tel/data schemes are skipped.
func ExtractLinks(htmlStr string, baseURL string) []LinkRef {
	if htmlStr == "" {
		return nil
	}
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}

	base, _ := url.Parse(baseURL)

	var out []LinkRef
	seen := map[string]bool{}

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Script, atom.Style:
				return
			case atom.A:
				if ref, ok := buildLinkRef(n, base); ok {
					key := ref.Href + "\x00" + ref.Text
					if !seen[key] {
						seen[key] = true
						out = append(out, ref)
					}
				}
				// Fall through: nested <a> is invalid HTML, but keep walking
				// so we don't miss valid descendants in malformed input.
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

// buildLinkRef extracts href/text/rel from an anchor node. Returns ok=false
// when the anchor should be skipped (no href, fragment-only, skipped scheme).
func buildLinkRef(n *html.Node, base *url.URL) (LinkRef, bool) {
	hrefRaw := strings.TrimSpace(getAttr(n, "href"))
	if hrefRaw == "" {
		return LinkRef{}, false
	}
	if hrefRaw == "#" || strings.HasPrefix(hrefRaw, "#") {
		return LinkRef{}, false
	}

	ref, err := url.Parse(hrefRaw)
	if err != nil {
		return LinkRef{}, false
	}
	if ref.Scheme != "" && skippedSchemes[strings.ToLower(ref.Scheme)] {
		return LinkRef{}, false
	}

	resolved := hrefRaw
	if base != nil {
		resolved = base.ResolveReference(ref).String()
	}

	return LinkRef{
		Href: resolved,
		Text: extractAnchorText(n),
		Rel:  strings.TrimSpace(getAttr(n, "rel")),
	}, true
}

// extractAnchorText walks descendants, concatenates TextNode contents,
// collapses whitespace runs, and truncates at linkTextMaxLen (with ellipsis).
// <script>/<style> subtrees are skipped.
func extractAnchorText(n *html.Node) string {
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Script, atom.Style:
				return
			}
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}

	collapsed := collapseWhitespace(b.String())
	if len([]rune(collapsed)) > linkTextMaxLen {
		runes := []rune(collapsed)
		collapsed = string(runes[:linkTextMaxLen]) + "…"
	}
	return collapsed
}

// collapseWhitespace trims leading/trailing whitespace and collapses internal
// whitespace runs (any mix of spaces/tabs/newlines) into a single space.
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
