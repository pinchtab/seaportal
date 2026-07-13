package engine

import (
	"fmt"
	"strings"
	"time"
)

func extractPDF(result *Result, targetURL string, opts Options, st *fetchState, start time.Time) {
	md, perr := ExtractPDFText(st.bodyBytes)
	if perr != nil {
		result.setError(fmt.Errorf("pdf extraction failed: %w", perr))
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
