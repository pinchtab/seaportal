package engine

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
	result.TraceInfo = computeTraceInfo(&result.ResponseHeaders)
	cdn := fingerprintCDN(&result.ResponseHeaders)
	cdn.ViaHops = parseViaHeader(result.ResponseVia)
	cdn.ProxyLayers = len(cdn.ViaHops)
	result.CDNInfo = cdn
	result.RequestAcceptEncoding = DefaultAcceptEncoding
	result.RequestID = opts.RequestID

	mergePreWarnings(result, st.preWarnings)
}

type postProcessConfig struct {
	linkRetention bool
	dedupe        bool
}

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

func applyChunkStage(content string, result *Result, opts Options) {
	if opts.Chunk.Strategy != ChunkOff {
		result.Chunks = ChunkMarkdown(content, opts.Chunk)
	}
}

func refreshContentMetrics(result *Result) {
	result.QualityInfo = ComputeQuality(result.Content)
	result.Quality = result.QualityInfo.Score
	result.Fingerprint = SemanticFingerprint(result.Content)
}
