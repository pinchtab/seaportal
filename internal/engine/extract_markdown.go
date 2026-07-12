package engine

import (
	"net/http"
	"time"
)

// extractNegotiatedMarkdown handles a body the server delivered as
// text/markdown (via `Accept: text/markdown` content negotiation): cleanup,
// the full post-content pipeline (link retention, dedupe, truncate, chunk),
// markdown-native structure counts, and the llms.txt discovery headers that
// only apply on this path.
func extractNegotiatedMarkdown(result *Result, opts Options, st *fetchState, html string, contentLength, fetchTimeMs int64, start time.Time) {
	content := CleanupMarkdown(html)
	content = applyContentPostProcessing(content, result, opts, postProcessConfig{linkRetention: true, dedupe: true})

	result.Content = content
	result.Length = len(content)
	result.TimeMs = time.Since(start).Milliseconds()
	result.FetchTimeMs = fetchTimeMs
	result.Title = extractMarkdownTitle(content)
	result.HeadingCount = CountMarkdownHeadings(content)
	result.LinkCount = CountMarkdownLinks(content)
	result.ParagraphCount = countMarkdownParagraphs(content)
	refreshContentMetrics(result)
	result.HasLLMContent = detectLLMContent(content)

	if st.resp.StatusCode == http.StatusOK {
		result.Confidence = 100
		result.Profile = PageProfile{
			Class:       PageSSR,
			Outcome:     OutcomeExtract,
			Reasons:     []string{"content-negotiation-markdown"},
			Confidence:  100,
			Trustworthy: true,
		}
	} else {
		result.Confidence = computeConfidence(confidenceInputs{length: result.Length, headingCount: result.HeadingCount, paragraphCount: result.ParagraphCount})
	}

	result.Validation = ValidateExtraction(result)

	// llms.txt discovery: only the markdown negotiation path inspects the
	// Link / X-LLMs-Txt response headers (kept from the pre-refactor copies).
	linkHeader := st.resp.Header.Get("Link")
	if linkHeader != "" {
		result.ResponseLink = linkHeader
		result.LLMsTxtURL = extractLLMsTxtURL(linkHeader)
	}
	xLLMsTxt := st.resp.Header.Get("X-LLMs-Txt")
	if xLLMsTxt != "" && result.LLMsTxtURL == "" {
		result.LLMsTxtURL = xLLMsTxt
	}

	finalizeTransport(result, opts, st, contentLength)

	applyStatusBlockedProfile(result, st.resp.StatusCode)

	applyQueryRanking(result, opts)
}
