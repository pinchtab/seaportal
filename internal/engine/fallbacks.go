package engine

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-shiori/go-readability"
)

func adoptContent(result *Result, content, method string) {
	result.Content = content
	result.Length = len(content)
	refreshContentMetrics(result)
	result.ExtractionMethod = method
}

func applyPruneFallback(result *Result, rawHTML string, parsedURL *url.URL, opts Options) bool {
	if len(result.Content) >= 500 || opts.NoPruneFallback {
		return false
	}
	pruned := PruneToContent(rawHTML)
	if pruned == rawHTML {
		return false
	}
	prunedArticle, prunedErr := readability.FromReader(strings.NewReader(pruned), parsedURL)
	if prunedErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("prune fallback: re-readability failed: %v", prunedErr))
		return false
	}
	prunedMD, mdErr := convertHTMLToMarkdown(prunedArticle.Content)
	if mdErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("prune fallback: markdown conversion failed: %v", mdErr))
		return false
	}
	prunedMD = CleanupMarkdown(prunedMD)
	if len(prunedMD) <= 2*len(result.Content) {
		return false
	}
	adoptContent(result, prunedMD, "prune-fallback")
	if result.Title == "" && prunedArticle.Title != "" {
		result.Title = prunedArticle.Title
	}
	if result.Byline == "" {
		result.Byline = prunedArticle.Byline
	}
	if result.Excerpt == "" {
		result.Excerpt = prunedArticle.Excerpt
	}
	if result.SiteName == "" {
		result.SiteName = prunedArticle.SiteName
	}
	result.HeadingCount = CountPattern(prunedArticle.Content, `<h[1-6]`)
	htmlLinks := CountPattern(prunedArticle.Content, `<a\s`)
	mdLinks := CountMarkdownLinks(prunedMD)
	if mdLinks > htmlLinks {
		result.LinkCount = mdLinks
	} else {
		result.LinkCount = htmlLinks
	}
	result.ParagraphCount = CountPattern(prunedArticle.Content, `<p[\s>]`)
	result.Validation = ValidateExtraction(result)
	result.PruneFallbackUsed = true
	return true
}

const preprocessSkipThinFloor = 8192

func applyPreprocessSkipFallback(result *Result, sanitizedHTML, rawHTML, targetURL string, parsedURL *url.URL, start, parseStart, parseEnd time.Time) {
	if result.Length == 0 || result.Length >= preprocessSkipThinFloor {
		return
	}
	if result.Length*2 >= visibleTextLenOf(sanitizedHTML) {
		return
	}
	altArticle, err := readability.FromReader(strings.NewReader(SanitizeHTML(rawHTML)), parsedURL)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("preprocess-skip fallback: readability failed: %v", err))
		return
	}
	alt := processArticle(altArticle, targetURL, start, parseStart, parseEnd)
	if alt.Length > result.Length*7/5 && alt.Length > result.Length+400 {
		alt.ExtractionMethod = "preprocess-skip-fallback"
		*result = alt
	}
}

func applyIndexPageFallback(result *Result, html string) bool {
	indexResult := DetectIndexPage(html)
	if !ShouldUseIndexFallback(*result, indexResult) {
		return false
	}
	adoptContent(result, indexResult.Markdown, "index-page")
	result.HeadingCount = indexResult.HeadlineCount
	result.LinkCount = CountMarkdownLinks(indexResult.Markdown)
	result.Confidence = indexResult.Confidence
	result.SPASignals = append(result.SPASignals, "index-page-fallback")
	return true
}

func applyLDJSONArticleBodyFallback(result *Result, ldBlocks []LDJSONBlock, opts Options) bool {
	if result.Length >= 500 || opts.NoPruneFallback || len(ldBlocks) == 0 {
		return false
	}
	for _, b := range ldBlocks {
		if b.Headline == "" || b.Body == "" {
			continue
		}
		body := strings.TrimSpace(b.Body)
		if len(body) < 2*result.Length+200 {
			continue
		}
		var bodyMD string
		if strings.Contains(body, "<") && strings.Contains(body, ">") {
			if md, mdErr := convertHTMLToMarkdown(body); mdErr == nil {
				bodyMD = CleanupMarkdown(md)
			} else {
				result.Warnings = append(result.Warnings, fmt.Sprintf("ld-json article body conversion: %v", mdErr))
				bodyMD = body
			}
		} else {
			bodyMD = body
		}
		if bodyMD == "" {
			continue
		}
		adoptContent(result, bodyMD, "json-ld-article-body")
		if result.Title == "" {
			result.Title = b.Headline
		}
		return true
	}
	return false
}

func applyTextFallback(result *Result, html string) bool {
	if result.Length >= 500 || result.IsBlocked || len(html) <= 10000 {
		return false
	}
	textResult := TextFallback(html)
	if textResult.Length <= result.Length || textResult.Length < 200 {
		return false
	}
	result.Content = textResult.Content
	result.Length = textResult.Length
	result.HeadingCount = textResult.Headings
	result.LinkCount = textResult.Links
	result.SPASignals = append(result.SPASignals, "text-fallback")
	result.Confidence = computeConfidence(confidenceInputsFrom(result))
	refreshContentMetrics(result)
	result.ExtractionMethod = "text-fallback"
	return true
}

func applyLDJSONSupplement(result *Result, ldBlocks []LDJSONBlock) bool {
	if len(ldBlocks) == 0 {
		return false
	}
	supplemented := false
	ldContent := LDJSONToMarkdown(ldBlocks)
	if ldContent != "" && result.Length < 5000 {
		if result.Content != "" {
			result.Content = result.Content + "\n\n---\n\n" + ldContent
		} else {
			result.Content = ldContent
		}
		result.Length = len(result.Content)
		result.SPASignals = append(result.SPASignals, "ldjson-supplemented")
		refreshContentMetrics(result)
		result.Confidence = computeConfidence(confidenceInputsFrom(result))
		supplemented = true
	}
	result.LDJSONBlocks = ldBlocks
	return supplemented
}
