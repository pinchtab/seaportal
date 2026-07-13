package engine

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type LinkRef struct {
	Href string `json:"href"`
	Text string `json:"text,omitempty"`
	Rel  string `json:"rel,omitempty"`
}

const linkTextMaxLen = 200

var skippedSchemes = map[string]bool{
	"javascript": true,
	"mailto":     true,
	"tel":        true,
	"data":       true,
}

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
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

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

func extractAnchorText(n *html.Node) string {
	collapsed := collapseWhitespace(nodeText(n, true))
	if len([]rune(collapsed)) > linkTextMaxLen {
		runes := []rune(collapsed)
		collapsed = string(runes[:linkTextMaxLen]) + "…"
	}
	return collapsed
}
