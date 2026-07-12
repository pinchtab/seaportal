package engine

// finalize.go — the shared result-finalization tails that were previously
// copy-pasted across the PDF / raw / markdown / HTML branches of
// FromURLWithOptions: the transport/telemetry stamp, the post-content
// pipeline (link retention → dedupe → truncate → chunk), and the quality
// metrics refresh. Per-branch divergence (TimeMs/FetchTimeMs semantics, the
// markdown branch's Link header handling, which stages run) stays at the
// call sites.

// mergePreWarnings prepends fetch-phase warnings onto result.Warnings.
// Prepending keeps fetch warnings (cache write, charset decode) ahead of
// extraction warnings (selectors, schema) in stable order. Safe to call
// with a nil or empty pre slice — no-op in that case.
func mergePreWarnings(result *Result, pre []string) {
	if len(pre) == 0 {
		return
	}
	if len(result.Warnings) == 0 {
		result.Warnings = append([]string(nil), pre...)
		return
	}
	merged := make([]string, 0, len(pre)+len(result.Warnings))
	merged = append(merged, pre...)
	merged = append(merged, result.Warnings...)
	result.Warnings = merged
}

// finalizeTransport stamps the shared transport/telemetry tail onto result:
// status, content length, timings, retry counters, redirect info, response
// headers, trace/CDN fingerprints, proxy hops, and the fetch-phase warnings.
func finalizeTransport(result *Result, opts Options, st *fetchState, contentLength int64) {
	resp := st.resp
	result.StatusCode = resp.StatusCode
	result.ContentLength = contentLength
	result.TTFBMs = st.ttfbMs
	result.DownloadMs = st.downloadMs
	result.RetryCount = st.retryCount
	result.TotalRetryWait = st.totalRetryWait
	result.HeadPreflightStatus = st.headPreflightStatus
	result.RedirectCount = st.tracker.count
	result.RedirectChain = st.tracker.chain
	if resp.Request != nil && resp.Request.URL != nil {
		result.FinalURL = resp.Request.URL.String()
	}

	populateResponseHeaders(result, resp)
	result.TraceFormats, result.TraceCorrelation = computeTraceInfo(*result)
	result.CDNProvider, result.CDNSignals = fingerprintCDN(*result)
	result.ViaHops = parseViaHeader(result.ResponseVia)
	result.ProxyLayers = len(result.ViaHops)
	result.RequestAcceptEncoding = DefaultAcceptEncoding
	result.RequestID = opts.RequestID

	mergePreWarnings(result, st.preWarnings)
}

// postProcessConfig selects which optional stages of the post-content
// pipeline run for a content branch: PDF runs link retention only, raw
// passthrough runs neither, negotiated markdown runs both.
type postProcessConfig struct {
	linkRetention bool
	dedupe        bool
}

// applyContentPostProcessing runs the shared post-content tail on content —
// link retention, dedupe, MaxTokens truncation, chunking, per cfg — recording
// side effects (dedupe stats, Truncated, Chunks) on result, and returns the
// transformed content. The HTML branch (fromHTMLInternal) calls the
// individual stages instead because other logic is interleaved between them.
func applyContentPostProcessing(content string, result *Result, opts Options, cfg postProcessConfig) string {
	if cfg.linkRetention {
		content, _ = applyLinkRetentionStage(content, opts)
	}
	if cfg.dedupe {
		content, _ = applyDedupeStage(content, result, opts)
	}
	content, _ = applyTruncateStage(content, result, opts)
	applyChunkStage(content, result, opts)
	return content
}

// applyLinkRetentionStage applies the resolved link-retention mode to
// content. Citations demote "all" to footer form. Reports whether a rewrite
// actually ran (mode "all", or empty content, leaves content untouched).
func applyLinkRetentionStage(content string, opts Options) (string, bool) {
	if content == "" {
		return content, false
	}
	mode := opts.LinkRetention
	if mode == LinkRetentionAll && opts.Citations {
		mode = LinkRetentionFooter
	}
	if mode == LinkRetentionAll {
		return content, false
	}
	return applyLinkRetention(content, mode), true
}

// applyDedupeStage runs block dedupe over content when opts.Dedupe is set,
// recording the dedupe statistics on result. Returns the deduped content and
// whether dedupe ran.
func applyDedupeStage(content string, result *Result, opts Options) (string, bool) {
	if !opts.Dedupe || content == "" {
		return content, false
	}
	dedupeOpts := DefaultDedupeOptions()
	if opts.NoNearDedupe {
		dedupeOpts.NearDup = false
	}
	dedupeResult := DedupeWithOptions(content, dedupeOpts)
	result.DedupeApplied = true
	result.DuplicatesRemoved = dedupeResult.DuplicatesFound
	result.DuplicateSignals = dedupeResult.DuplicateSignals
	result.NearDuplicatesRemoved = dedupeResult.NearDuplicatesFound
	result.NearDuplicateSignals = dedupeResult.NearDuplicateSignals
	result.OriginalBlockCount = dedupeResult.OriginalBlocks
	result.UniqueBlockCount = dedupeResult.UniqueBlocks
	return dedupeResult.Content, true
}

// applyTruncateStage truncates content to opts.MaxTokens at a paragraph
// boundary, setting result.Truncated when a cut happened.
func applyTruncateStage(content string, result *Result, opts Options) (string, bool) {
	if opts.MaxTokens <= 0 {
		return content, false
	}
	truncated, didTrunc := TruncateMarkdownAtParagraph(content, opts.MaxTokens)
	if !didTrunc {
		return content, false
	}
	result.Truncated = true
	return truncated, true
}

// applyChunkStage populates result.Chunks when chunking is enabled.
func applyChunkStage(content string, result *Result, opts Options) {
	if opts.Chunk.Strategy != ChunkOff {
		result.Chunks = ChunkMarkdown(content, opts.Chunk)
	}
}

// refreshContentMetrics recomputes the quality score and semantic
// fingerprint from result.Content. Call after any stage that replaces or
// rewrites the content.
func refreshContentMetrics(result *Result) {
	result.QualityInfo = ComputeQuality(result.Content)
	result.Quality = result.QualityInfo.Score
	result.Fingerprint = SemanticFingerprint(result.Content)
}
