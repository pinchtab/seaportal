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
	// Coverage notes. The dense-pattern rec targets "many similar URLs per
	// pattern → sample them". Its density gate has a blind spot: a site with
	// sparse clustering (many patterns, e.g. multi-locale kubernetes.io at
	// 6453 URLs / 941 patterns) AND heavy undersampling got no signal at all.
	// The low-coverage fallback fires on the sampled fraction alone (< 25%),
	// independent of how URLs cluster (ALP-045).
	undersampled := totalURLsInSitemap > 0 && totalURLsInSitemap > len(pages)
	switch {
	case undersampled && patternCount > 0 && totalURLsInSitemap/patternCount >= 10:
		recs = append(recs, fmt.Sprintf("sitemap lists %d URLs across %d patterns but only %d were sampled; sampling recommended", totalURLsInSitemap, patternCount, len(pages)))
	case undersampled && len(pages)*4 < totalURLsInSitemap:
		pct := float64(len(pages)) / float64(totalURLsInSitemap) * 100
		recs = append(recs, fmt.Sprintf("only %d of %d sitemap URLs were sampled (%.1f%% coverage); raise --max-pages for broader coverage", len(pages), totalURLsInSitemap, pct))
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
