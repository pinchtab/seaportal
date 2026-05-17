package engine

// ldjson_metadata.go — Lift Article/NewsArticle/BlogPosting JSON-LD fields onto
// the structured Result. JSON-LD wins over readability byline and meta tags
// (per the user story): we want trustworthy publisher-asserted metadata to
// supersede heuristic extraction.

// applyLDJSONMetadata picks the first Article-shaped block (one with a non-
// empty Headline — BreadcrumbList/WebSite/Organization don't carry headlines)
// and populates Result.Byline, Result.PublishedDate, Result.Language,
// Result.Section unconditionally when the corresponding block field is set.
//
// Wired into extract.go between the readability assignments and the meta-tag
// fallback (applyMetaAuthors), so the priority chain becomes:
//
//	JSON-LD > citation_author meta > readability > empty.
func applyLDJSONMetadata(result *Result, blocks []LDJSONBlock) {
	if result == nil || len(blocks) == 0 {
		return
	}

	var article *LDJSONBlock
	for i := range blocks {
		if blocks[i].Headline != "" {
			article = &blocks[i]
			break
		}
	}
	if article == nil {
		return
	}

	if article.Author != "" {
		result.Byline = article.Author
	}
	if article.DatePub != "" {
		result.PublishedDate = article.DatePub
	}
	if article.Language != "" {
		result.Language = article.Language
	}
	if article.Section != "" {
		result.Section = article.Section
	}
}
