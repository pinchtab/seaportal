package engine

import (
	"strings"
	"time"
)

// extractPDF routes the fetched bytes through ExtractPDFText and reuses the
// same post-content pipeline (link retention, truncation, chunking) as the
// markdown path. HTML-specific stages (readability, dedupe, prune fallback,
// JSON-LD fallback) are skipped — they don't apply to PDFs.
func extractPDF(result *Result, targetURL string, opts Options, st *fetchState, start time.Time) {
	md, perr := ExtractPDFText(st.bodyBytes)
	if perr != nil {
		result.Error = "pdf extraction failed: " + perr.Error()
		result.StatusCode = st.resp.StatusCode
		result.ContentLength = int64(len(st.bodyBytes))
		result.ResponseContentType = st.respContentType
		return
	}

	content := applyContentPostProcessing(md, result, opts, postProcessConfig{linkRetention: true})

	result.URL = targetURL
	result.Content = content
	result.Length = len(content)
	result.ExtractionMethod = "pdf"
	result.TimeMs = time.Since(start).Milliseconds()
	result.FetchTimeMs = time.Since(start).Milliseconds()

	// Fallback title: first non-empty line that isn't a "--- page N ---" marker.
	if result.Title == "" {
		for _, l := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(l)
			if trimmed == "" || strings.HasPrefix(trimmed, "--- page ") {
				continue
			}
			result.Title = trimmed
			break
		}
	}

	refreshContentMetrics(result)
	result.HeadingCount = 0
	result.LinkCount = CountMarkdownLinks(content)
	result.ParagraphCount = countMarkdownParagraphs(content)
	result.Confidence = 90
	result.Profile = PageProfile{
		Class:       PageStatic,
		Outcome:     OutcomeExtract,
		Reasons:     []string{"pdf-extracted"},
		Confidence:  90,
		Trustworthy: true,
	}
	result.PageClass = PageStatic
	result.Validation = ValidateExtraction(result)

	finalizeTransport(result, opts, st, int64(len(st.bodyBytes)))
}
