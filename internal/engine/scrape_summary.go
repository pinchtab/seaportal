package engine

import (
	"fmt"
	"sort"
	"strings"
)

const thinContentThreshold = 160

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
