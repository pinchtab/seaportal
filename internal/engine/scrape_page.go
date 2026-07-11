package engine

import (
	"encoding/json"
	"net/url"
	"strings"
)

// assemblePage turns a fetched/extracted page into a fully-populated PageObject.
// It wires the existing extractors (metadata, JSON-LD, links) over the raw HTML
// rather than recomputing, and carries the markdown/title/status from the
// extraction Result. Internal vs external links are counted relative to base.
// Performance (TTFB, total bytes, request count) is filled only when
// withPerformance is set. A failed page (r.Error != "") still yields a
// PageObject with its status and error.
func assemblePage(base *url.URL, targetURL, htmlStr string, r Result, withPerformance bool) PageObject {
	p := PageObject{
		URL:      targetURL,
		Title:    r.Title,
		Status:   r.StatusCode,
		Markdown: r.Content,
		Error:    r.Error,
	}

	meta := ExtractMetadata(htmlStr)
	p.Meta = metaMap(r, meta)

	blocks := ExtractLDJSON(htmlStr)
	p.Schema = ldjsonToMaps(blocks)

	p.InternalLinks, p.ExternalLinks = countLinks(base, ExtractLinks(htmlStr, targetURL))
	p.ContentType = classifyContentType(blocks, meta, targetURL, htmlStr)

	if withPerformance {
		p.Performance = &PagePerformance{
			TTFBMillis: r.TTFBMs,
			TotalBytes: int64(len(htmlStr)),
			Requests:   1,
		}
	}
	return p
}

// metaMap flattens the extracted Metadata (falling back to Result fields) into
// the PageObject.Meta string map, dropping empty values.
func metaMap(r Result, m Metadata) map[string]string {
	out := map[string]string{}
	put := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	put("title", r.Title)
	put("description", firstNonEmpty(m.Description, r.Description))
	put("author", firstNonEmpty(m.Author, r.Byline))
	put("publishedDate", firstNonEmpty(m.PublishedDate, r.PublishedDate))
	put("language", firstNonEmpty(m.Language, r.Language))
	put("section", firstNonEmpty(m.Section, r.Section))
	put("image", firstNonEmpty(m.ImageURL, r.ImageURL))
	put("siteName", r.SiteName)
	put("ogType", m.OGType)
	put("keywords", m.Keywords)
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

// classifyContentType derives a coarse content type from JSON-LD @type, then
// OpenGraph type. When neither is present it falls back to a structural
// heuristic over the URL and HTML (ALP-041) so metadata-poor sites (e.g. MDN)
// don't collapse to "unknown"; a genuinely empty body still classifies as
// "unknown".
func classifyContentType(blocks []LDJSONBlock, m Metadata, pageURL, html string) string {
	for _, b := range blocks {
		if t := normalizeContentType(b.Type); t != "" {
			return t
		}
	}
	if t := normalizeContentType(m.OGType); t != "" {
		return t
	}
	return structuralContentType(pageURL, html)
}

// articleURLSegments are path segments that strongly signal long-form content.
var articleURLSegments = []string{"/blog", "/docs", "/article", "/post", "/news", "/guide", "/tutorial"}

// structuralContentType infers a coarse type from URL shape and HTML structure
// for pages that carry no JSON-LD/OpenGraph type. A page is "article" when its
// URL sits under a docs/blog-style segment, or the HTML has a main <article>
// element, or it reads as dense prose (a heading plus several paragraphs).
// Anything else with a usable body is "page"; an empty body stays "unknown".
func structuralContentType(pageURL, html string) string {
	if strings.TrimSpace(html) == "" {
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
	lower := strings.ToLower(html)
	if strings.Contains(lower, "<article") {
		return "article"
	}
	headings := strings.Count(lower, "<h1") + strings.Count(lower, "<h2") + strings.Count(lower, "<h3")
	if headings >= 1 && strings.Count(lower, "<p") >= 5 {
		return "article"
	}
	return "page"
}

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
