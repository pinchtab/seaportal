package engine

import (
	"encoding/json"
	"net/url"
	"strings"
)

// assemblePage turns an extraction Result into a fully-populated PageObject.
// On the converged fetch path (T07) the page's raw HTML is no longer in hand,
// so everything derives from the Result: the metadata fields the pipeline
// resolved, the LD-JSON blocks it recorded, the opt-in captured link list
// (Options.WithLinks), and its structural metrics for content-type
// classification. Internal vs external links are counted relative to base.
// Performance (TTFB, fetched bytes, request count) is filled only when
// withPerformance is set. A failed page (r.Error != "") still yields a
// PageObject with its status and error.
func assemblePage(base *url.URL, targetURL string, r Result, withPerformance bool) PageObject {
	p := PageObject{
		URL:      targetURL,
		Title:    r.Title,
		Status:   r.StatusCode,
		Markdown: r.Content,
		Error:    r.Error,
	}

	p.Meta = metaMap(r)
	p.Schema = ldjsonToMaps(r.LDJSONBlocks)
	p.InternalLinks, p.ExternalLinks = countLinks(base, r.Links)
	p.ContentType = classifyContentType(r, targetURL)

	if withPerformance {
		p.Performance = &PagePerformance{
			TTFBMillis: r.TTFBMs,
			TotalBytes: r.ContentLength,
			Requests:   1,
		}
	}
	return p
}

// metaMap flattens the Result's resolved metadata (JSON-LD first, <meta>
// fill-when-empty — see applyLDJSONMetadata/applyMetadata) into the
// PageObject.Meta string map, dropping empty values.
func metaMap(r Result) map[string]string {
	out := map[string]string{}
	put := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	put("title", r.Title)
	put("description", r.Description)
	put("author", r.Byline)
	put("publishedDate", r.PublishedDate)
	put("language", r.Language)
	put("section", r.Section)
	put("image", r.ImageURL)
	put("siteName", r.SiteName)
	if len(out) == 0 {
		return nil
	}
	return out
}

// ldjsonToMaps renders each JSON-LD block as a generic map (empty fields
// dropped via the block's omitempty tags) for the PageObject.Schema slice.
func ldjsonToMaps(blocks []LDJSONBlock) []map[string]any {
	var out []map[string]any
	for _, b := range blocks {
		data, err := json.Marshal(b)
		if err != nil {
			continue
		}
		var m map[string]any
		if json.Unmarshal(data, &m) == nil && len(m) > 0 {
			out = append(out, m)
		}
	}
	return out
}

// countLinks splits links into internal (same host as base, or relative) vs
// external counts.
func countLinks(base *url.URL, links []LinkRef) (internal, external int) {
	for _, l := range links {
		u, err := url.Parse(l.Href)
		if err != nil {
			continue
		}
		if u.Host == "" || (base != nil && strings.EqualFold(u.Host, base.Host)) {
			internal++
		} else {
			external++
		}
	}
	return internal, external
}

// classifyContentType derives a coarse content type from the extraction
// Result: JSON-LD @type first, then a structural heuristic over the URL shape
// and the extractor's own metrics (ALP-041) so metadata-poor sites (e.g. MDN)
// don't collapse to "unknown". A page with no extractable content classifies
// as "unknown". (On the converged fetch path the raw HTML is not retained, so
// og:type-only pages without JSON-LD fall through to the structural
// heuristics — T07.)
func classifyContentType(r Result, pageURL string) string {
	for _, b := range r.LDJSONBlocks {
		if t := normalizeContentType(b.Type); t != "" {
			return t
		}
	}
	if strings.TrimSpace(r.Content) == "" {
		return "unknown"
	}
	if u, err := url.Parse(pageURL); err == nil {
		path := strings.ToLower(u.Path)
		for _, seg := range articleURLSegments {
			if strings.Contains(path, seg) {
				return "article"
			}
		}
	}
	// Dense prose (a heading plus several paragraphs, as counted by the
	// extraction pipeline) reads as an article.
	if r.HeadingCount >= 1 && r.ParagraphCount >= 5 {
		return "article"
	}
	return "page"
}

// articleURLSegments are path segments that strongly signal long-form content.
var articleURLSegments = []string{"/blog", "/docs", "/article", "/post", "/news", "/guide", "/tutorial"}

// normalizeContentType maps a schema.org / OpenGraph type onto one of a small
// set of categories, or "" when unrecognized (so the caller can fall through).
func normalizeContentType(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "" {
		return ""
	}
	if i := strings.LastIndexAny(t, "/#"); i >= 0 {
		t = t[i+1:]
	}
	switch t {
	case "article", "newsarticle", "blogposting", "techarticle", "report":
		return "article"
	case "product", "productgroup", "offer":
		return "product"
	case "itemlist", "collectionpage", "breadcrumblist", "searchresultspage":
		return "listing"
	case "webpage", "website", "aboutpage", "contactpage":
		return "page"
	default:
		return ""
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
