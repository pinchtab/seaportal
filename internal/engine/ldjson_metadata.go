package engine

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
