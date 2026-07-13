package engine

import "time"

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

	finalizeTransport(result, opts, st, int64(len(st.bodyBytes)))
}
