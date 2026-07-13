package engine

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type ImageRef struct {
	Src    string `json:"src"`
	Alt    string `json:"alt,omitempty"`
	Srcset string `json:"srcset,omitempty"`
	Title  string `json:"title,omitempty"`
}

const imageAltMaxLen = 200

func ExtractImages(htmlStr string, baseURL string) []ImageRef {
	if htmlStr == "" {
		return nil
	}
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}

	base, _ := url.Parse(baseURL)

	var out []ImageRef
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
			case atom.Img:
				if ref, ok := buildImageRef(n, base); ok {
					if !seen[ref.Src] {
						seen[ref.Src] = true
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

func buildImageRef(n *html.Node, base *url.URL) (ImageRef, bool) {
	srcRaw := strings.TrimSpace(getAttr(n, "src"))
	if srcRaw == "" {
		return ImageRef{}, false
	}
	if strings.HasPrefix(strings.ToLower(srcRaw), "data:") {
		return ImageRef{}, false
	}

	resolved := srcRaw
	if base != nil {
		if ref, err := url.Parse(srcRaw); err == nil {
			resolved = base.ResolveReference(ref).String()
		}
	}

	alt := collapseWhitespace(getAttr(n, "alt"))
	if len([]rune(alt)) > imageAltMaxLen {
		runes := []rune(alt)
		alt = string(runes[:imageAltMaxLen]) + "…"
	}

	return ImageRef{
		Src:    resolved,
		Alt:    alt,
		Srcset: strings.TrimSpace(getAttr(n, "srcset")),
		Title:  strings.TrimSpace(getAttr(n, "title")),
	}, true
}
