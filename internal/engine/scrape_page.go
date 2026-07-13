package engine

import (
	"encoding/json"
	"net/url"
	"strings"
)

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
	if r.HeadingCount >= 1 && r.ParagraphCount >= 5 {
		return "article"
	}
	return "page"
}

var articleURLSegments = []string{"/blog", "/docs", "/article", "/post", "/news", "/guide", "/tutorial"}

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
