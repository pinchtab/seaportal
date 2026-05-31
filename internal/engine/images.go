package engine

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ImageRef is a structured representation of a discovered <img> element in the
// raw page HTML. Surfaced on Result.Images when the caller opts in via
// Options.WithImages so agents can pick a cover image, fetch alt-text for
// accessibility, or download referenced assets without re-parsing the HTML.
type ImageRef struct {
	Src    string `json:"src"`
	Alt    string `json:"alt,omitempty"`
	Srcset string `json:"srcset,omitempty"`
	Title  string `json:"title,omitempty"`
}

const imageAltMaxLen = 200

// ExtractImages walks htmlStr and returns every <img src="…"> as an ImageRef in
// document order. Relative src values are resolved against baseURL; entries
// are deduplicated by src (first occurrence wins — alt/srcset from the first
// hit are kept). Images inside <script>/<style> subtrees are skipped, as are
// empty src and data: URLs (typically noisy inline-encoded SVGs/PNGs).
//
// V1 scope: only the <img> element's own src/srcset/alt/title are read.
// Known limitations (deferred to V2):
//   - <picture><source srcset=…> siblings are NOT flattened; only the <img>
//     fallback is captured (its own src/srcset is the agreed default).
//   - data-src / data-srcset lazy-load attributes are NOT consulted; sites
//     that put real URLs only in data-src will surface placeholder src values.
//   - <img> inside <noscript> is invisible because the HTML5 parser treats
//     <noscript> content as text.
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
				// Fall through: <img> is a void element so there's nothing to
				// recurse into, but stay symmetric with the links walker.
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

// buildImageRef extracts src/alt/srcset/title from an <img> node. Returns
// ok=false when the image should be skipped (no src, data: URL).
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
