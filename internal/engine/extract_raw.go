package engine

import "time"

// extractRawText passes non-HTML text bodies — JSON/XML and their +json/+xml
// variants — through verbatim, mirroring the PDF bypass. The HTML→markdown
// conversion escapes markdown metacharacters (`_`, `[`, …), which corrupts
// structured text — e.g. a JSON body's "node_id" becomes the invalid escape
// "node\_id" (ALP-036). Skipping readability/dedupe/markdown keeps the body
// byte-for-byte (subject only to opt-in truncation/chunking).
func extractRawText(result *Result, targetURL string, opts Options, st *fetchState, start time.Time) {
	content := applyContentPostProcessing(string(st.bodyBytes), result, opts, postProcessConfig{})

	result.URL = targetURL
	result.Content = content
	result.Length = len(content)
	result.ExtractionMethod = "raw"
	result.TimeMs = time.Since(start).Milliseconds()
	result.FetchTimeMs = time.Since(start).Milliseconds()

	refreshContentMetrics(result)
	result.Confidence = 90
	result.Profile = PageProfile{
		Class:       PageStatic,
		Outcome:     OutcomeExtract,
		Reasons:     []string{"raw-passthrough"},
		Confidence:  90,
		Trustworthy: true,
	}
	result.PageClass = PageStatic
	result.Validation = ValidateExtraction(result)

	// Divergence kept from the pre-refactor copies: unlike the PDF branch,
	// raw passthrough never populates HeadingCount/LinkCount/ParagraphCount.
	finalizeTransport(result, opts, st, int64(len(st.bodyBytes)))
}
