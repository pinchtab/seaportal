package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RenderScrapeJSON marshals a ScrapeResult to indented JSON matching the spec's
// output structure.
func RenderScrapeJSON(res *ScrapeResult) ([]byte, error) {
	if res == nil {
		return nil, fmt.Errorf("seaportal: nil ScrapeResult")
	}
	return json.MarshalIndent(res, "", "  ")
}

// RenderScrapeMarkdown renders a ScrapeResult as a single human-readable
// Markdown digest: a site header, per-group sections, per-page content, and a
// summary.
func RenderScrapeMarkdown(res *ScrapeResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder

	title := res.Site.Title
	if title == "" {
		title = res.Site.BaseURL
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- Base URL: %s\n", res.Site.BaseURL)
	fmt.Fprintf(&b, "- Sitemap found: %t\n", res.Site.SitemapFound)
	if res.Site.TotalURLsInSitemap > 0 {
		fmt.Fprintf(&b, "- URLs in sitemap: %d\n", res.Site.TotalURLsInSitemap)
	}
	fmt.Fprintf(&b, "- Sampled pages: %d\n\n", res.Site.SampledPages)

	if len(res.PageGroups) > 0 {
		b.WriteString("## Page groups\n\n")
		for _, g := range res.PageGroups {
			fmt.Fprintf(&b, "- `%s` — sampled %d of %d\n", g.Pattern, g.Sampled, g.TotalInSitemap)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Pages\n\n")
	for _, p := range res.Pages {
		heading := p.Title
		if heading == "" {
			heading = p.URL
		}
		fmt.Fprintf(&b, "### %s\n\n", heading)
		fmt.Fprintf(&b, "- URL: %s\n", p.URL)
		fmt.Fprintf(&b, "- Status: %d", p.Status)
		if p.ContentType != "" {
			fmt.Fprintf(&b, " · Type: %s", p.ContentType)
		}
		b.WriteString("\n")
		if p.Error != "" {
			fmt.Fprintf(&b, "- Error: %s\n", p.Error)
		}
		b.WriteString("\n")
		if p.Markdown != "" {
			b.WriteString(p.Markdown)
			b.WriteString("\n\n")
		}
	}

	b.WriteString("## Summary\n\n")
	if len(res.Summary.ContentTypes) > 0 {
		b.WriteString("Content types:\n")
		for _, k := range contentTypesSorted(res.Summary.ContentTypes) {
			fmt.Fprintf(&b, "- %s: %d\n", k, res.Summary.ContentTypes[k])
		}
		b.WriteString("\n")
	}
	if len(res.Summary.Recommendations) > 0 {
		b.WriteString("Recommendations:\n")
		for _, r := range res.Summary.Recommendations {
			fmt.Fprintf(&b, "- %s\n", r)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// WriteScrapeDirectory writes a ScrapeResult to dir: result.json, one
// pages/<slug>.md per page (filesystem-safe, collision-free slugs), and an
// index.md manifest mapping each URL to its file. Returns the relative page
// file paths in page order.
func WriteScrapeDirectory(res *ScrapeResult, dir string) ([]string, error) {
	if res == nil {
		return nil, fmt.Errorf("seaportal: nil ScrapeResult")
	}
	pagesDir := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		return nil, err
	}

	data, err := RenderScrapeJSON(res)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), data, 0o644); err != nil {
		return nil, err
	}

	used := map[string]bool{}
	relPaths := make([]string, 0, len(res.Pages))
	var manifest strings.Builder
	fmt.Fprintf(&manifest, "# %s — pages\n\n", firstNonEmpty(res.Site.Title, res.Site.BaseURL))

	for _, p := range res.Pages {
		slug := uniqueSlug(slugForURL(p.URL), used)
		rel := filepath.Join("pages", slug+".md")
		relPaths = append(relPaths, rel)

		var pb strings.Builder
		fmt.Fprintf(&pb, "# %s\n\n", firstNonEmpty(p.Title, p.URL))
		fmt.Fprintf(&pb, "- URL: %s\n- Status: %d\n", p.URL, p.Status)
		if p.ContentType != "" {
			fmt.Fprintf(&pb, "- Type: %s\n", p.ContentType)
		}
		if p.Error != "" {
			fmt.Fprintf(&pb, "- Error: %s\n", p.Error)
		}
		pb.WriteString("\n")
		pb.WriteString(p.Markdown)
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(pb.String()), 0o644); err != nil {
			return nil, err
		}
		fmt.Fprintf(&manifest, "- [%s](%s) — %s\n", firstNonEmpty(p.Title, p.URL), rel, p.URL)
	}

	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(manifest.String()), 0o644); err != nil {
		return nil, err
	}
	return relPaths, nil
}

// slugForURL builds a filesystem-safe slug from a URL's path (falling back to
// "index" for the root).
func slugForURL(raw string) string {
	p := pathOf(raw)
	p = strings.Trim(p, "/")
	if p == "" {
		return "index"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(p) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "index"
	}
	const maxSlug = 80
	if len(slug) > maxSlug {
		slug = strings.Trim(slug[:maxSlug], "-")
	}
	return slug
}

// uniqueSlug returns slug, or slug-2, slug-3… if already used.
func uniqueSlug(slug string, used map[string]bool) string {
	candidate := slug
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s-%d", slug, i)
	}
	used[candidate] = true
	return candidate
}
