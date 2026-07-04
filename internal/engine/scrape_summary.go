package engine

import (
	"fmt"
	"sort"
	"strings"
)

// thinContentThreshold is the extracted-markdown length below which a 200-OK
// page is treated as thin / likely SPA/JS-only for recommendation purposes.
const thinContentThreshold = 160

// summarize rolls the assembled page objects up into the ScrapeResult summary:
// a contentType tally plus heuristic, text-only recommendations. It makes no
// network calls and never returns a nil ContentTypes map.
func summarize(pages []PageObject, totalURLsInSitemap, patternCount int) ScrapeSummary {
	contentTypes := map[string]int{}
	errors, thin := 0, 0
	for _, p := range pages {
		ct := p.ContentType
		if ct == "" {
			ct = "unknown"
		}
		contentTypes[ct]++

		if p.Error != "" || p.Status >= 400 {
			errors++
			continue
		}
		if p.Status == 200 && len(strings.TrimSpace(p.Markdown)) < thinContentThreshold {
			thin++
		}
	}

	var recs []string
	if errors > 0 {
		recs = append(recs, fmt.Sprintf("%d of %d pages returned errors or 4xx/5xx responses", errors, len(pages)))
	}
	if thin > 0 {
		recs = append(recs, fmt.Sprintf("%d pages have little extractable text (possible SPA/JS-only); consider PinchTab enrichment", thin))
	}
	if totalURLsInSitemap > 0 && patternCount > 0 && totalURLsInSitemap > len(pages) && totalURLsInSitemap/patternCount >= 10 {
		recs = append(recs, fmt.Sprintf("sitemap lists %d URLs across %d patterns but only %d were sampled; sampling recommended", totalURLsInSitemap, patternCount, len(pages)))
	}

	return ScrapeSummary{
		ContentTypes:    contentTypes,
		Recommendations: recs,
	}
}

// contentTypesSorted returns the contentType keys in stable order (highest
// count first, then alphabetical) — handy for deterministic rendering.
func contentTypesSorted(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}
