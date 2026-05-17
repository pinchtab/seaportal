package engine

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	mdtable "github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/andybalholm/brotli"
	"github.com/go-shiori/go-readability"
	"github.com/klauspost/compress/zstd"
)

var (
	mdConverterOnce sync.Once
	mdConverter     *converter.Converter
)

// getMarkdownConverter lazy-initialises the html-to-markdown converter so
// short-lived invocations (--version, --help, subcommands that never extract
// HTML) skip the plugin construction cost. First HTML extraction pays once.
func getMarkdownConverter() *converter.Converter {
	mdConverterOnce.Do(func() {
		mdConverter = converter.NewConverter(
			converter.WithPlugins(
				base.NewBasePlugin(),
				commonmark.NewCommonmarkPlugin(),
				mdtable.NewTablePlugin(),
			),
		)
	})
	return mdConverter
}

func convertHTMLToMarkdown(html string) (string, error) {
	return getMarkdownConverter().ConvertString(html)
}

func FromURL(targetURL string) Result {
	return FromURLWithOptions(targetURL, Options{})
}

func FromURLWithDedupe(targetURL string) Result {
	return FromURLWithOptions(targetURL, Options{Dedupe: true})
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Must match a real browser exactly — Cloudflare blocks truncated/incomplete UAs.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

const DefaultAcceptEncoding = "gzip, deflate, br, zstd"

const DefaultAccept = "text/markdown, text/html;q=0.9, application/xhtml+xml;q=0.8, application/xml;q=0.7, */*;q=0.1"

func newGETRequest(targetURL string, userAgent string, requestID string, sendRequestID bool) *http.Request {
	req, _ := http.NewRequest("GET", targetURL, nil)
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", DefaultAccept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", DefaultAcceptEncoding)
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="122", "Not(A:Brand";v="24", "Google Chrome";v="122"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	if sendRequestID && requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	return req
}

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

func FromURLWithOptions(targetURL string, opts Options) (result Result) {
	if opts.HeadOnly {
		return fetchHeadOnly(targetURL, opts)
	}
	defer ensureProfile(&result)
	start := time.Now()
	result = Result{URL: targetURL}

	// data: URL short-circuit (RFC 2397). Bypasses the network entirely:
	// decode inline body and feed it straight into the HTML pipeline. Scope
	// is strictly text/html + text/plain; other media types (binary/image)
	// error cleanly so we don't pipe non-text bytes through readability.
	if strings.HasPrefix(targetURL, "data:") {
		mime, body, derr := parseDataURL(targetURL)
		if derr != nil {
			result.Error = derr.Error()
			return result
		}
		switch mime {
		case "text/html", "text/plain":
			return fromHTMLInternal(string(body), targetURL, start, opts)
		default:
			result.Error = "data: URL mime not supported: " + mime
			return result
		}
	}

	// preWarnings collects warnings raised during fetch/decode (before
	// fromHTMLInternal replaces `result`). They are merged into
	// result.Warnings via mergePreWarnings before each return path.
	var preWarnings []string

	domain := extractDomain(targetURL)

	timeout := 30 * time.Second
	if opts.DomainTimeout != nil && domain != "" {
		if domainTimeout, ok := opts.DomainTimeout[domain]; ok && domainTimeout > 0 {
			timeout = domainTimeout
		}
	}

	tracker := &redirectTracker{}

	sharedC, clientErr := getClientForOptions(opts)
	if clientErr != nil {
		result.Error = "invalid proxy URL: " + clientErr.Error()
		return result
	}

	var client *http.Client
	if opts.NoPooling || (opts.DomainTimeout != nil && domain != "" && opts.DomainTimeout[domain] > 0) {
		client = &http.Client{Timeout: timeout, CheckRedirect: tracker.checkRedirect}
		// Honour proxy even in the no-pooling / per-domain-timeout branch.
		if opts.Proxy != "" {
			client.Transport = sharedC.Transport
		}
	} else {
		client = &http.Client{
			Timeout:       sharedC.Timeout,
			Transport:     sharedC.Transport,
			CheckRedirect: tracker.checkRedirect,
		}
	}
	// Test injection: an opts-supplied RoundTripper trumps the utls/proxy
	// transport. Lets mock.Replay/mock.Record serve canned bytes without
	// touching the network.
	if opts.Transport != nil {
		client.Transport = opts.Transport
	}

	userAgent := DefaultUserAgent
	if opts.UserAgent != "" {
		userAgent = ResolveUserAgent(opts.UserAgent)
	}
	if opts.DomainUserAgent != nil && domain != "" {
		if ua, ok := opts.DomainUserAgent[domain]; ok && ua != "" {
			userAgent = ua
		}
	}

	maxRetries := opts.MaxRetries
	maxRetryWait := opts.MaxRetryWait
	if opts.DomainRetryConfig != nil && domain != "" {
		if domainCfg, ok := opts.DomainRetryConfig[domain]; ok {
			if domainCfg.MaxRetries > 0 {
				maxRetries = domainCfg.MaxRetries
			}
			if domainCfg.MaxRetryWait > 0 {
				maxRetryWait = domainCfg.MaxRetryWait
			}
		}
	}

	if maxRetryWait == 0 {
		maxRetryWait = 60 * time.Second
	}

	totalRetryTimeout := opts.TotalRetryTimeout
	if totalRetryTimeout == 0 {
		totalRetryTimeout = 120 * time.Second
	}

	var headPreflightStatus int
	if opts.HeadPreflight || opts.ContentTypePreflight {
		headReq, _ := http.NewRequest("HEAD", targetURL, nil)
		headReq.Header.Set("User-Agent", userAgent)
		headResp, headErr := client.Do(headReq)
		if headErr == nil {
			headPreflightStatus = headResp.StatusCode
			contentType := headResp.Header.Get("Content-Type")
			_ = headResp.Body.Close()

			if opts.ContentTypePreflight && headPreflightStatus == http.StatusOK {
				if isBinaryContentType(contentType) {
					result.HeadPreflightStatus = headPreflightStatus
					result.Error = fmt.Sprintf("skipped binary content: %s", contentType)
					return result
				}
			}

			if opts.HeadPreflight {
				if headPreflightStatus == http.StatusNotFound || headPreflightStatus == http.StatusGone {
					result.HeadPreflightStatus = headPreflightStatus
					result.Error = fmt.Sprintf("HEAD preflight returned %d", headPreflightStatus)
					return result
				}
			}
		}
	}

	if opts.RespectRobots && domain != "" {
		cache := opts.CrawlDelayCache
		if cache == nil {
			cache = NewCrawlDelayCache()
		}
		if parsed, perr := url.Parse(targetURL); perr == nil {
			scheme := parsed.Scheme
			if scheme == "" {
				scheme = "https"
			}
			host := parsed.Host
			if host == "" {
				host = domain
			}
			if !cache.IsAllowed(host, userAgent, scheme, parsed.RequestURI()) {
				result.Error = "blocked by robots.txt"
				result.BlockedByRobots = true
				ensureProfile(&result)
				result.Profile.Reasons = append(result.Profile.Reasons, "blocked-by-robots")
				return result
			}
		}
	}

	if opts.RespectCrawlDelay && domain != "" {
		crawlCache := opts.CrawlDelayCache
		if crawlCache == nil {
			crawlCache = NewCrawlDelayCache()
		}
		// Use parsed.Host (host[:port]) instead of domain (hostname only) so the
		// cache fetches robots.txt from the right port and keys per-port. Mirrors
		// the RespectRobots branch above. Without this, non-default-port targets
		// (httptest, local dev, intranet :8080) silently get no delay enforcement.
		scheme := "https"
		host := domain
		if parsedURL, err := url.Parse(targetURL); err == nil {
			if parsedURL.Scheme != "" {
				scheme = parsedURL.Scheme
			}
			if parsedURL.Host != "" {
				host = parsedURL.Host
			}
		}
		if delay := crawlCache.GetDelayWithScheme(host, userAgent, scheme); delay > 0 {
			time.Sleep(delay)
		}
	}

	if opts.RateLimit > 0 && domain != "" {
		limiter := opts.RateLimiter
		if limiter == nil {
			limiter = NewHostRateLimiter()
		}
		limiter.Wait(domain, opts.RateLimit)
	}

	req := newGETRequest(targetURL, userAgent, opts.RequestID, opts.SendRequestID)

	// On-disk content cache (opt-in via opts.CacheDir). Read-side is bypassed
	// by opts.NoCache; the write-side still records fresh 200s so --no-cache
	// behaves as "force refresh" rather than "disable cache entirely".
	var cache *DiskCache
	if opts.CacheDir != "" {
		c, cacheErr := NewDiskCache(opts.CacheDir, opts.CacheTTL)
		if cacheErr != nil {
			fmt.Fprintf(os.Stderr, "warning: cache init failed: %v\n", cacheErr)
		} else {
			cache = c
		}
	}

	var retryCount int
	var totalRetryWait time.Duration

	logRetry := func(event RetryEvent) {
		if opts.RetryLogger != nil {
			opts.RetryLogger(event)
		}
	}

	var resp *http.Response
	var cacheHitResp bool
	// pendingRevalidation holds the stale cache entry whose body should be
	// replayed if the conditional GET returns 304. Captured *before* we mutate
	// req with conditional headers so the key stays stable for TouchByKey.
	var pendingRevalidationMeta *cachedResponse
	var pendingRevalidationBody []byte
	var pendingCacheKey string
	if cache != nil && !opts.NoCache {
		meta, body, fresh, swrStale, beyondTolerance := cache.GetStaleWithTolerance(targetURL, req, opts.CacheStaleTolerance)
		if fresh {
			// Synthesize an *http.Response so the existing decompress/charset/
			// extract pipeline consumes the cached entry identically to a live
			// fetch. The retry loop is skipped entirely on a cache hit.
			resp = &http.Response{
				Status:     fmt.Sprintf("%d %s", meta.Status, http.StatusText(meta.Status)),
				StatusCode: meta.Status,
				Header:     meta.Headers.Clone(),
				Body:       io.NopCloser(bytes.NewReader(body)),
				Request:    req,
			}
			result.CacheHit = true
			cacheHitResp = true
		} else if swrStale {
			// SWR band: serve the cached body immediately, fire the
			// conditional GET (or fresh GET if no validators) in a
			// background goroutine. The caller's latency is body-replay only.
			resp = &http.Response{
				Status:     fmt.Sprintf("%d %s", meta.Status, http.StatusText(meta.Status)),
				StatusCode: meta.Status,
				Header:     meta.Headers.Clone(),
				Body:       io.NopCloser(bytes.NewReader(body)),
				Request:    req,
			}
			result.CacheStale = true
			cacheHitResp = true
			bgKey := cache.cacheKey(targetURL, req)
			spawnBackgroundRefresh(cache, bgKey, targetURL, meta, userAgent, opts)
		} else if beyondTolerance {
			// Past TTL+tolerance but the cached entry carries ETag/Last-Modified.
			// Snapshot the key BEFORE adding conditional headers so TouchByKey
			// targets the original entry (conditional headers don't participate
			// in cacheKey today, but capturing first is safer).
			pendingCacheKey = cache.cacheKey(targetURL, req)
			pendingRevalidationMeta = meta
			pendingRevalidationBody = body
			for k, v := range meta.ConditionalHeaders() {
				req.Header.Set(k, v)
			}
		}
	}

	for attempt := 0; !cacheHitResp && attempt <= maxRetries; attempt++ {
		var err error
		resp, err = client.Do(req)
		if err != nil {
			if attempt < maxRetries && isRetryableError(err) {
				backoff := time.Duration(1<<uint(attempt)) * time.Second
				if backoff > maxRetryWait {
					backoff = maxRetryWait
				}
				backoff = addJitter(backoff)
				if totalRetryWait+backoff > totalRetryTimeout {
					logRetry(RetryEvent{Attempt: attempt + 1, Error: err, WaitTime: backoff, Outcome: "timeout"})
					result.Error = err.Error()
					result.RetryCount = retryCount
					result.TotalRetryWait = totalRetryWait
					return result
				}
				logRetry(RetryEvent{Attempt: attempt + 1, Error: err, WaitTime: backoff, Outcome: "retrying"})
				time.Sleep(backoff)
				retryCount++
				totalRetryWait += backoff
				req = newGETRequest(targetURL, userAgent, opts.RequestID, opts.SendRequestID)
				continue
			}
			logRetry(RetryEvent{Attempt: attempt + 1, Error: err, Outcome: "exhausted"})
			result.Error = err.Error()
			result.RetryCount = retryCount
			result.TotalRetryWait = totalRetryWait
			return result
		}

		if (resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout || (resp.StatusCode == http.StatusServiceUnavailable && resp.Header.Get("Retry-After") == "")) && attempt < maxRetries {
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			if backoff > maxRetryWait {
				backoff = maxRetryWait
			}
			backoff = addJitter(backoff)
			if totalRetryWait+backoff > totalRetryTimeout {
				logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, WaitTime: backoff, Outcome: "timeout"})
				break
			}
			logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, WaitTime: backoff, Outcome: "retrying"})
			_ = resp.Body.Close()
			time.Sleep(backoff)
			retryCount++
			totalRetryWait += backoff
			req = newGETRequest(targetURL, userAgent, opts.RequestID, opts.SendRequestID)
			continue
		}

		if (resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusServiceUnavailable && resp.Header.Get("Retry-After") != "")) && attempt < maxRetries {
			retryAfterHeader := resp.Header.Get("Retry-After")
			if retryAfterHeader == "" {
				logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, Outcome: "exhausted"})
				break
			}
			retryAfter, ok := parseRetryAfter(retryAfterHeader)
			if !ok || retryAfter > maxRetryWait {
				logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, WaitTime: retryAfter, Outcome: "exhausted"})
				break
			}
			if totalRetryWait+retryAfter > totalRetryTimeout {
				logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, WaitTime: retryAfter, Outcome: "timeout"})
				break
			}
			logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, WaitTime: retryAfter, Outcome: "retrying"})
			_ = resp.Body.Close()
			time.Sleep(retryAfter)
			retryCount++
			totalRetryWait += retryAfter
			req = newGETRequest(targetURL, userAgent, opts.RequestID, opts.SendRequestID)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, Outcome: "success"})
		}
		break
	}
	defer func() { _ = resp.Body.Close() }()

	result.Protocol = negotiatedProtocol(req, resp)

	ttfbMs := time.Since(start).Milliseconds()

	// 304 Not Modified for a stale cache entry: replay the cached body and
	// refresh FetchedAt so subsequent calls see it as fresh.
	if pendingRevalidationMeta != nil && resp.StatusCode == http.StatusNotModified {
		_ = resp.Body.Close()
		resp = &http.Response{
			Status:     fmt.Sprintf("%d %s", pendingRevalidationMeta.Status, http.StatusText(pendingRevalidationMeta.Status)),
			StatusCode: pendingRevalidationMeta.Status,
			Header:     pendingRevalidationMeta.Headers.Clone(),
			Body:       io.NopCloser(bytes.NewReader(pendingRevalidationBody)),
			Request:    req,
		}
		if cache != nil && pendingCacheKey != "" {
			if touchErr := cache.TouchByKey(pendingCacheKey); touchErr != nil {
				preWarnings = append(preWarnings, fmt.Sprintf("cache touch: %v", touchErr))
			}
		}
		result.CacheRevalidated = true
	}

	downloadStart := time.Now()
	bodyBytes, err := io.ReadAll(resp.Body)
	downloadMs := time.Since(downloadStart).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Persist successful 200 OK responses to the on-disk cache. Cache the raw
	// (still-encoded) wire bytes so replay flows through the same decompress
	// path as a live fetch. Errors are non-fatal: log and continue. Skip when
	// we just replayed a 304 (CacheRevalidated) — TouchByKey already handled it.
	if cache != nil && !result.CacheHit && !result.CacheRevalidated && !result.CacheStale && resp.StatusCode == http.StatusOK {
		if putErr := cache.Put(targetURL, req, resp.StatusCode, resp.Header, bodyBytes); putErr != nil {
			preWarnings = append(preWarnings, fmt.Sprintf("cache write: %v", putErr))
		}
	}

	// Go's http.Client only auto-decompresses when it adds Accept-Encoding itself.
	// We set Accept-Encoding manually, so we must decompress.
	// Fallback path: HTTP/2 transport may have already decompressed despite the header.
	contentEncoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
	if contentEncoding != "" {
		decompressed, decompErr := decompressBody(bodyBytes, contentEncoding)
		if decompErr != nil {
			if len(bodyBytes) > 0 {
				trimmed := bytes.TrimSpace(bodyBytes)
				if len(trimmed) > 0 && (trimmed[0] == '<' || trimmed[0] == '{') {
				} else {
					result.Error = fmt.Sprintf("decompression error (%s): %v", contentEncoding, decompErr)
					return result
				}
			} else {
				result.Error = fmt.Sprintf("decompression error (%s): %v", contentEncoding, decompErr)
				return result
			}
		} else {
			bodyBytes = decompressed
		}
	}

	respContentType := resp.Header.Get("Content-Type")
	if isCharsetSniffableContentType(respContentType) {
		if decoded, cs, ok := sniffAndDecode(bodyBytes, respContentType); ok {
			bodyBytes = decoded
			result.Charset = cs
		} else if declared := detectCharset(bodyBytes, respContentType); declared != "" {
			// A charset was declared but decode failed — fall through to raw
			// bytes (extraction usually still works on UTF-8-shaped input).
			preWarnings = append(preWarnings, fmt.Sprintf("charset decode: declared charset %q not recognised, falling back to raw bytes", declared))
		}
	}

	// PDF branch: when the response is application/pdf and the caller hasn't
	// opted out via --no-pdf, route the bytes through ExtractPDFText and reuse
	// the same post-content pipeline (link retention, truncation, chunking)
	// as the markdown path. HTML-specific stages (readability, dedupe, prune
	// fallback, JSON-LD fallback) are skipped — they don't apply to PDFs.
	ctLower := strings.ToLower(respContentType)
	if strings.Contains(ctLower, "application/pdf") {
		if opts.NoPDF {
			result.Error = fmt.Sprintf("skipped binary content: %s", respContentType)
			result.StatusCode = resp.StatusCode
			result.ContentLength = int64(len(bodyBytes))
			result.ResponseContentType = respContentType
			result.TimeMs = time.Since(start).Milliseconds()
			result.FetchTimeMs = time.Since(start).Milliseconds()
			result.TTFBMs = ttfbMs
			result.DownloadMs = downloadMs
			return result
		}
		md, perr := ExtractPDFText(bodyBytes)
		if perr != nil {
			result.Error = "pdf extraction failed: " + perr.Error()
			result.StatusCode = resp.StatusCode
			result.ContentLength = int64(len(bodyBytes))
			result.ResponseContentType = respContentType
			return result
		}

		content := md

		if content != "" {
			mode := opts.LinkRetention
			if mode == LinkRetentionAll && opts.Citations {
				mode = LinkRetentionFooter
			}
			if mode != LinkRetentionAll {
				content = applyLinkRetention(content, mode)
			}
		}

		if opts.MaxTokens > 0 {
			truncated, didTrunc := TruncateMarkdownAtParagraph(content, opts.MaxTokens)
			if didTrunc {
				content = truncated
				result.Truncated = true
			}
		}

		if opts.Chunk.Strategy != ChunkOff {
			result.Chunks = ChunkMarkdown(content, opts.Chunk)
		}

		result.URL = targetURL
		result.Content = content
		result.Length = len(content)
		result.ExtractionMethod = "pdf"
		result.StatusCode = resp.StatusCode
		result.ContentLength = int64(len(bodyBytes))
		result.ResponseContentType = respContentType
		result.TimeMs = time.Since(start).Milliseconds()
		result.FetchTimeMs = time.Since(start).Milliseconds()
		result.TTFBMs = ttfbMs
		result.DownloadMs = downloadMs
		result.RetryCount = retryCount
		result.TotalRetryWait = totalRetryWait
		result.HeadPreflightStatus = headPreflightStatus
		result.RedirectCount = tracker.count
		result.RedirectChain = tracker.chain
		if resp.Request != nil && resp.Request.URL != nil {
			result.FinalURL = resp.Request.URL.String()
		}

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

		result.QualityInfo = ComputeQuality(content)
		result.Quality = result.QualityInfo.Score
		result.Fingerprint = SemanticFingerprint(content)
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
		result.Validation = ValidateExtraction(&result)

		populateResponseHeaders(&result, resp)
		result.TraceFormats, result.TraceCorrelation = computeTraceInfo(result)
		result.CDNProvider, result.CDNSignals = fingerprintCDN(result)
		result.ViaHops = parseViaHeader(result.ResponseVia)
		result.ProxyLayers = len(result.ViaHops)
		result.RequestAcceptEncoding = DefaultAcceptEncoding
		result.RequestID = opts.RequestID

		mergePreWarnings(&result, preWarnings)
		return result
	}

	html := string(bodyBytes)
	contentLength := int64(len(bodyBytes))
	fetchTimeMs := time.Since(start).Milliseconds()
	// Some servers reject `Accept: text/markdown` outright: Next.js etc. respond
	// 404, spec-compliant servers respond 406. Retry HTML-only so we still get a
	// usable body. Skip when the body actually is markdown.
	negotiationFailed := resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNotAcceptable
	if negotiationFailed && !strings.Contains(respContentType, "text/markdown") {
		_ = resp.Body.Close()
		req = newGETRequest(targetURL, userAgent, opts.RequestID, opts.SendRequestID)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
		resp, err = client.Do(req)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		defer func() { _ = resp.Body.Close() }()

		bodyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		contentEncoding = strings.ToLower(resp.Header.Get("Content-Encoding"))
		if contentEncoding != "" {
			decompressed, decompErr := decompressBody(bodyBytes, contentEncoding)
			if decompErr == nil {
				bodyBytes = decompressed
			}
		}
		respContentType = resp.Header.Get("Content-Type")
		if isCharsetSniffableContentType(respContentType) {
			if decoded, cs, ok := sniffAndDecode(bodyBytes, respContentType); ok {
				bodyBytes = decoded
				result.Charset = cs
			} else if declared := detectCharset(bodyBytes, respContentType); declared != "" {
				preWarnings = append(preWarnings, fmt.Sprintf("charset decode: declared charset %q not recognised, falling back to raw bytes", declared))
			}
		}
		html = string(bodyBytes)
		contentLength = int64(len(bodyBytes))
		fetchTimeMs = time.Since(start).Milliseconds()
	}

	if strings.Contains(respContentType, "text/markdown") {
		content := CleanupMarkdown(html)

		if content != "" {
			mode := opts.LinkRetention
			if mode == LinkRetentionAll && opts.Citations {
				mode = LinkRetentionFooter
			}
			if mode != LinkRetentionAll {
				content = applyLinkRetention(content, mode)
				// result.Length is computed below from len(content) — no extra refresh needed.
			}
		}

		if opts.Dedupe && content != "" {
			dedupeOpts := DefaultDedupeOptions()
			if opts.NoNearDedupe {
				dedupeOpts.NearDup = false
			}
			dedupeResult := DedupeWithOptions(content, dedupeOpts)
			content = dedupeResult.Content
			result.DedupeApplied = true
			result.DuplicatesRemoved = dedupeResult.DuplicatesFound
			result.DuplicateSignals = dedupeResult.DuplicateSignals
			result.NearDuplicatesRemoved = dedupeResult.NearDuplicatesFound
			result.NearDuplicateSignals = dedupeResult.NearDuplicateSignals
			result.OriginalBlockCount = dedupeResult.OriginalBlocks
			result.UniqueBlockCount = dedupeResult.UniqueBlocks
		}

		if opts.MaxTokens > 0 {
			truncated, didTrunc := TruncateMarkdownAtParagraph(content, opts.MaxTokens)
			if didTrunc {
				content = truncated
				result.Truncated = true
			}
		}

		if opts.Chunk.Strategy != ChunkOff {
			result.Chunks = ChunkMarkdown(content, opts.Chunk)
		}

		result.Content = content
		result.Length = len(content)
		result.TimeMs = time.Since(start).Milliseconds()
		result.FetchTimeMs = fetchTimeMs
		result.StatusCode = resp.StatusCode
		result.ContentLength = contentLength
		result.ResponseContentType = respContentType
		result.Title = extractMarkdownTitle(content)
		result.HeadingCount = CountMarkdownHeadings(content)
		result.LinkCount = CountMarkdownLinks(content)
		result.ParagraphCount = countMarkdownParagraphs(content)
		result.QualityInfo = ComputeQuality(content)
		result.Quality = result.QualityInfo.Score
		result.Fingerprint = SemanticFingerprint(content)
		result.HasLLMContent = detectLLMContent(content)

		if resp.StatusCode == http.StatusOK {
			result.Confidence = 100
			result.Profile = PageProfile{
				Class:       PageSSR,
				Outcome:     OutcomeExtract,
				Reasons:     []string{"content-negotiation-markdown"},
				Confidence:  100,
				Trustworthy: true,
			}
		} else {
			result.Confidence = ComputeConfidence(result.Length, result.HeadingCount, result.ParagraphCount, 0, false)
		}

		result.Validation = ValidateExtraction(&result)

		linkHeader := resp.Header.Get("Link")
		if linkHeader != "" {
			result.ResponseLink = linkHeader
			result.LLMsTxtURL = extractLLMsTxtURL(linkHeader)
		}
		xLLMsTxt := resp.Header.Get("X-LLMs-Txt")
		if xLLMsTxt != "" && result.LLMsTxtURL == "" {
			result.LLMsTxtURL = xLLMsTxt
		}

		populateResponseHeaders(&result, resp)
		result.TraceFormats, result.TraceCorrelation = computeTraceInfo(result)
		result.CDNProvider, result.CDNSignals = fingerprintCDN(result)
		result.ViaHops = parseViaHeader(result.ResponseVia)
		result.ProxyLayers = len(result.ViaHops)
		result.RequestAcceptEncoding = DefaultAcceptEncoding
		result.RequestID = opts.RequestID
		result.TTFBMs = ttfbMs
		result.DownloadMs = downloadMs
		result.RetryCount = retryCount
		result.TotalRetryWait = totalRetryWait
		result.HeadPreflightStatus = headPreflightStatus
		result.RedirectCount = tracker.count
		result.RedirectChain = tracker.chain
		if resp.Request != nil && resp.Request.URL != nil {
			result.FinalURL = resp.Request.URL.String()
		}

		applyStatusBlockedProfile(&result, resp.StatusCode)

		applyQueryRanking(&result, opts)

		mergePreWarnings(&result, preWarnings)
		return result
	}

	if opts.FastMode {
		needsBrowser, reason := QuickNeedsBrowser(html)
		if needsBrowser {
			result.TimeMs = time.Since(start).Milliseconds()
			result.FetchTimeMs = fetchTimeMs
			result.StatusCode = resp.StatusCode
			result.ContentLength = contentLength
			result.Validation = Validation{
				IsValid:      false,
				NeedsBrowser: true,
				Confidence:   0.1,
				Issues:       []string{reason},
			}
			result.Error = "needs-browser: " + reason
			applyStatusBlockedProfile(&result, resp.StatusCode)
			return result
		}
	}

	detectedCharset := result.Charset
	cacheHit := result.CacheHit
	cacheRevalidated := result.CacheRevalidated
	cacheStale := result.CacheStale
	protocol := result.Protocol
	result = fromHTMLInternal(html, targetURL, start, opts)
	result.Charset = detectedCharset
	result.CacheHit = cacheHit
	result.CacheRevalidated = cacheRevalidated
	result.CacheStale = cacheStale
	result.Protocol = protocol
	result.FetchTimeMs = fetchTimeMs
	result.TTFBMs = ttfbMs
	result.DownloadMs = downloadMs
	result.StatusCode = resp.StatusCode
	result.RetryCount = retryCount
	result.TotalRetryWait = totalRetryWait
	result.HeadPreflightStatus = headPreflightStatus
	result.ContentLength = contentLength
	result.RedirectCount = tracker.count
	result.RedirectChain = tracker.chain

	if resp.Request != nil && resp.Request.URL != nil {
		result.FinalURL = resp.Request.URL.String()
	}

	populateResponseHeaders(&result, resp)

	result.TraceFormats, result.TraceCorrelation = computeTraceInfo(result)

	result.CDNProvider, result.CDNSignals = fingerprintCDN(result)

	result.ViaHops = parseViaHeader(result.ResponseVia)
	result.ProxyLayers = len(result.ViaHops)

	result.RequestAcceptEncoding = DefaultAcceptEncoding
	result.RequestID = opts.RequestID

	if resp.StatusCode == http.StatusOK {
		isSoft404, soft404Hints := detectSoft404(html, contentLength)
		result.IsSoft404 = isSoft404
		result.Soft404Hints = soft404Hints
	}

	applyStatusBlockedProfile(&result, resp.StatusCode)

	mergePreWarnings(&result, preWarnings)
	return result
}

func applyStatusBlockedProfile(result *Result, statusCode int) {
	// Reasons that also flip the page to `blocked` + escalate to needs-browser.
	var blockedReason string
	switch statusCode {
	case http.StatusUnauthorized:
		blockedReason = "http-401-unauthorized"
	case http.StatusForbidden:
		blockedReason = "http-403-forbidden"
	case http.StatusTooManyRequests:
		blockedReason = "http-429-rate-limited"
	case http.StatusBadGateway:
		blockedReason = "http-502-bad-gateway"
	case http.StatusServiceUnavailable:
		blockedReason = "http-503-service-unavailable"
	case http.StatusGatewayTimeout:
		blockedReason = "http-504-gateway-timeout"
	}
	if blockedReason != "" {
		result.IsBlocked = true
		result.Profile.Class = PageBlocked
		result.Profile.Outcome = OutcomeNeedsBrowser
		result.Profile.Trustworthy = false
		result.Profile.Reasons = append(result.Profile.Reasons, blockedReason)
		result.PageClass = PageBlocked
		return
	}
	// Honest-but-not-blocked HTTP errors: add a reason for caller observability
	// without flipping IsBlocked. 404 = wrong URL (a browser won't help).
	// 500-599 (non-502/503/504) = server transient — retries handled upstream;
	// just label the final outcome.
	switch {
	case statusCode == http.StatusNotFound:
		result.Profile.Reasons = append(result.Profile.Reasons, "http-404-not-found")
	case statusCode >= 500 && statusCode < 600:
		result.Profile.Reasons = append(result.Profile.Reasons, "http-5xx-server-error")
	}
}

func FromHTML(html string, targetURL string) Result {
	start := time.Now()
	return fromHTMLInternal(html, targetURL, start, Options{})
}

// FromHTMLWithOptions runs the full extraction pipeline on pre-fetched HTML
// with custom Options. Same as FromHTML, but flag-aware — use this when the
// caller has already obtained the body (e.g. piped from a browser fetcher)
// and wants links/citations/strip/etc. honoured.
func FromHTMLWithOptions(html string, targetURL string, opts Options) Result {
	start := time.Now()
	return fromHTMLInternal(html, targetURL, start, opts)
}

func FromResponse(resp *http.Response, targetURL string, start time.Time) (result Result) {
	defer ensureProfile(&result)
	result = Result{URL: targetURL}

	parsedURL, _ := url.Parse(targetURL)
	parseStart := time.Now()
	article, err := readability.FromReader(resp.Body, parsedURL)
	parseEnd := time.Now()
	if err != nil {
		result.Error = err.Error()
		return result
	}

	return processArticle(article, targetURL, start, parseStart, parseEnd)
}

func fromHTMLInternal(html string, targetURL string, start time.Time, opts Options) (result Result) {
	defer ensureProfile(&result)

	// User-supplied CSS scoping runs first so every downstream pass — canonical
	// pick, link/image extraction, preprocess, readability — sees the
	// already-scoped DOM. --strip is applied before --select inside
	// applySelectorOps.
	var selectorWarnings []string
	if opts.SelectCSS != "" || opts.StripCSS != "" {
		modified, warns := applySelectorOps(html, opts.SelectCSS, opts.StripCSS)
		html = modified
		selectorWarnings = warns
	}

	spaSignals, isSPA := DetectSPA(html)
	isBlocked := DetectBlocked(html)
	// 200-OK + JS-challenge: small HTML bodies that ship a CDN/anti-bot
	// challenge instead of real content. DetectBlocked already covers most
	// CF/captcha pages via title/JS-variable patterns, but cross-cutting
	// signatures (cf-mitigated, datadome, perimeterx, etc.) bound by a
	// 1500-byte cap catch the rest without hostname-specific code.
	if !isBlocked && DetectJSChallenge(html, "text/html", len(html)) {
		isBlocked = true
		spaSignals = append(spaSignals, "js-challenge-200ok")
	}

	parsedURL, _ := url.Parse(targetURL)

	// Capture canonical signal from the original HTML before preprocessing/
	// sanitization can strip the <link rel="canonical"> tag.
	canonicalPick := PickCanonical(targetURL, html)

	// Capture the raw outbound-link list before chrome-stripping/sanitization
	// nukes nav/footer anchors. Gated by opt-in flag — link-heavy pages would
	// otherwise bloat output.
	var extractedLinks []LinkRef
	if opts.WithLinks {
		extractedLinks = ExtractLinks(html, targetURL)
	}

	// Same raw-HTML hook for images: capture before sanitize strips chrome
	// <img> (logos, social icons). Gated by opt-in flag to keep token usage
	// tight on image-heavy pages.
	var extractedImages []ImageRef
	if opts.WithImages {
		extractedImages = ExtractImages(html, targetURL)
	}

	// Same raw-HTML hook for tables: capture data-table structure before
	// preprocess unwraps layout tables and sanitize/readability rewrites
	// the table DOM. Gated by opt-in flag.
	var extractedTables []TableRef
	if opts.WithTables {
		extractedTables = ExtractTables(html, targetURL)
	}

	// Same raw-HTML hook for comments: capture user-generated comment
	// containers before preprocess strips them from main content. Gated by
	// opt-in flag — the strip still runs unconditionally so Content stays
	// clean either way.
	var extractedComments []CommentRef
	if opts.WithComments {
		extractedComments = ExtractComments(html, targetURL)
	}

	// Same raw-HTML hook for the declarative CSS schema. Schema runs on the
	// pre-preprocess DOM so caller-supplied selectors can target chrome
	// elements (nav/sidebar/footer) that the main pipeline strips. Load
	// failure and selector errors degrade to warnings, never crash.
	var extractedSchema map[string]interface{}
	var schemaWarnings []string
	schema := opts.Schema
	if schema == nil && opts.SchemaPath != "" {
		s, err := LoadSchema(opts.SchemaPath)
		if err != nil {
			schemaWarnings = append(schemaWarnings, "schema load failed: "+err.Error())
		} else {
			schema = &s
		}
	}
	if schema != nil {
		extracted, err := ApplySchema(html, *schema)
		if err != nil {
			schemaWarnings = append(schemaWarnings, "schema apply failed: "+err.Error())
		} else {
			extractedSchema = extracted
		}
	}

	// Snapshot the pre-preprocess HTML so the prune-fallback can run a
	// tag-density heuristic against the unscoped DOM if readability fails.
	rawHTML := html

	html = PreprocessHTMLWithURL(html, parsedURL)

	ldBlocks := ExtractLDJSON(html)

	pageMetadata := ExtractMetadata(html)

	html = SanitizeHTML(html)
	parseStart := time.Now()
	article, err := readability.FromReader(strings.NewReader(html), parsedURL)
	parseEnd := time.Now()
	if err != nil {
		result = Result{URL: targetURL, Error: err.Error(), SPASignals: spaSignals, IsSPA: isSPA, IsBlocked: isBlocked}
		return result
	}

	result = processArticle(article, targetURL, start, parseStart, parseEnd)

	// Prune-fallback rescue: when readability returned a thin article and the
	// caller hasn't opted out, retry on a tag-density-pruned DOM. Adopt the
	// rescue only when it more than doubles the markdown length — guards
	// against the heuristic preferring a noisy block over genuinely-tiny prose.
	if len(result.Content) < 500 && !opts.NoPruneFallback {
		pruned := PruneToContent(rawHTML)
		if pruned != rawHTML {
			prunedArticle, prunedErr := readability.FromReader(strings.NewReader(pruned), parsedURL)
			if prunedErr != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("prune fallback: re-readability failed: %v", prunedErr))
			}
			if prunedErr == nil {
				prunedMD, mdErr := convertHTMLToMarkdown(prunedArticle.Content)
				if mdErr != nil {
					result.Warnings = append(result.Warnings, fmt.Sprintf("prune fallback: markdown conversion failed: %v", mdErr))
				}
				if mdErr == nil {
					prunedMD = CleanupMarkdown(prunedMD)
					if len(prunedMD) > 2*len(result.Content) {
						result.Content = prunedMD
						result.Length = len(prunedMD)
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
						result.QualityInfo = ComputeQuality(prunedMD)
						result.Quality = result.QualityInfo.Score
						result.Fingerprint = SemanticFingerprint(prunedMD)
						result.Validation = ValidateExtraction(&result)
						result.PruneFallbackUsed = true
						result.ExtractionMethod = "prune-fallback"
					}
				}
			}
		}
	}

	result.SPASignals = spaSignals
	result.IsSPA = isSPA
	result.IsBlocked = isBlocked

	// JSON-LD Article metadata wins over readability + meta-tag fallbacks.
	applyLDJSONMetadata(&result, ldBlocks)

	// Unified <meta> pass fills-when-empty so JSON-LD priority is preserved.
	applyMetadata(&result, pageMetadata)

	// Tail-fallback: stopword-frequency language detection on the extracted
	// content. Only fires when metadata produced nothing AND there's enough
	// prose to vote on. Metadata-derived Language always wins.
	if result.Language == "" && len(result.Content) > 200 {
		result.Language = DetectLanguage(result.Content)
	}

	result.Confidence = ComputeConfidence(result.Length, result.HeadingCount, result.ParagraphCount, len(result.SPASignals), result.IsBlocked)

	indexResult := DetectIndexPage(html)
	if ShouldUseIndexFallback(result, indexResult) {
		result.Content = indexResult.Markdown
		result.Length = len(indexResult.Markdown)
		result.HeadingCount = indexResult.HeadlineCount
		result.LinkCount = CountMarkdownLinks(indexResult.Markdown)
		result.Confidence = indexResult.Confidence
		result.SPASignals = append(result.SPASignals, "index-page-fallback")
		result.QualityInfo = ComputeQuality(result.Content)
		result.Quality = result.QualityInfo.Score
		result.Fingerprint = SemanticFingerprint(result.Content)
		result.ExtractionMethod = "index-page"
	}

	// JSON-LD `articleBody` as primary candidate: when readability+prune is thin
	// AND any LD block ships a substantial body, replace content with it.
	// Reuses --no-prune-fallback as the opt-out gate (same intent: "no automatic
	// rescue"). Runs BEFORE text-fallback so a clean JSON-LD body wins over
	// generic text extraction.
	if result.Length < 500 && !opts.NoPruneFallback && len(ldBlocks) > 0 {
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
			result.Content = bodyMD
			result.Length = len(bodyMD)
			if result.Title == "" {
				result.Title = b.Headline
			}
			result.QualityInfo = ComputeQuality(result.Content)
			result.Quality = result.QualityInfo.Score
			result.Fingerprint = SemanticFingerprint(result.Content)
			result.ExtractionMethod = "json-ld-article-body"
			break
		}
	}

	if result.Length < 500 && !result.IsBlocked && len(html) > 10000 {
		textResult := TextFallback(html)
		if textResult.Length > result.Length && textResult.Length >= 200 {
			result.Content = textResult.Content
			result.Length = textResult.Length
			result.HeadingCount = textResult.Headings
			result.LinkCount = textResult.Links
			result.SPASignals = append(result.SPASignals, "text-fallback")
			result.Confidence = ComputeConfidence(result.Length, result.HeadingCount, result.ParagraphCount, len(result.SPASignals), result.IsBlocked)
			result.QualityInfo = ComputeQuality(result.Content)
			result.Quality = result.QualityInfo.Score
			result.Fingerprint = SemanticFingerprint(result.Content)
			result.ExtractionMethod = "text-fallback"
		}
	}

	if len(ldBlocks) > 0 {
		ldContent := LDJSONToMarkdown(ldBlocks)
		if ldContent != "" && result.Length < 5000 {
			if result.Content != "" {
				result.Content = result.Content + "\n\n---\n\n" + ldContent
			} else {
				result.Content = ldContent
			}
			result.Length = len(result.Content)
			result.SPASignals = append(result.SPASignals, "ldjson-supplemented")
			result.QualityInfo = ComputeQuality(result.Content)
			result.Quality = result.QualityInfo.Score
			result.Fingerprint = SemanticFingerprint(result.Content)
			result.Confidence = ComputeConfidence(result.Length, result.HeadingCount, result.ParagraphCount, len(result.SPASignals), result.IsBlocked)
		}
		result.LDJSONBlocks = ldBlocks
	}

	if result.Confidence < 30 {
		result.IsSPA = true
	}

	if result.Content != "" {
		mode := opts.LinkRetention
		if mode == LinkRetentionAll && opts.Citations {
			mode = LinkRetentionFooter
		}
		if mode != LinkRetentionAll {
			result.Content = applyLinkRetention(result.Content, mode)
			result.Length = len(result.Content)
		}
	}

	if opts.Dedupe && result.Content != "" {
		dedupeOpts := DefaultDedupeOptions()
		if opts.NoNearDedupe {
			dedupeOpts.NearDup = false
		}
		dedupeResult := DedupeWithOptions(result.Content, dedupeOpts)
		result.Content = dedupeResult.Content
		result.Length = len(result.Content)
		result.DedupeApplied = true
		result.DuplicatesRemoved = dedupeResult.DuplicatesFound
		result.DuplicateSignals = dedupeResult.DuplicateSignals
		result.NearDuplicatesRemoved = dedupeResult.NearDuplicatesFound
		result.NearDuplicateSignals = dedupeResult.NearDuplicateSignals
		result.OriginalBlockCount = dedupeResult.OriginalBlocks
		result.UniqueBlockCount = dedupeResult.UniqueBlocks

		result.QualityInfo = ComputeQuality(result.Content)
		result.Quality = result.QualityInfo.Score
		result.Fingerprint = SemanticFingerprint(result.Content)
	}

	result.HasLLMContent = detectLLMContent(result.Content)

	result.Profile = ClassifyPage(result)

	applyProbeSearchOverride(&result, opts)

	if opts.FailFast && result.IsSPA && result.Confidence < 30 {
		result.Error = fmt.Sprintf("SPA detected with low confidence (%d%%), signals: %v", result.Confidence, result.SPASignals)
	}

	if canonicalPick != "" && canonicalPick != result.URL {
		result.CanonicalURL = canonicalPick
	}

	if opts.WithLinks {
		result.Links = extractedLinks
	}

	if opts.WithImages {
		result.Images = extractedImages
	}

	if opts.WithTables {
		result.Tables = extractedTables
	}

	if opts.WithComments {
		result.Comments = extractedComments
	}

	if len(selectorWarnings) > 0 {
		result.Warnings = append(result.Warnings, selectorWarnings...)
	}

	if extractedSchema != nil {
		result.Schema = extractedSchema
	}
	if len(schemaWarnings) > 0 {
		result.Warnings = append(result.Warnings, schemaWarnings...)
	}

	if opts.MaxTokens > 0 && result.Content != "" {
		truncated, didTrunc := TruncateMarkdownAtParagraph(result.Content, opts.MaxTokens)
		if didTrunc {
			result.Content = truncated
			result.Truncated = true
			result.Length = len(result.Content)
		}
	}

	if opts.Chunk.Strategy != ChunkOff {
		result.Chunks = ChunkMarkdown(result.Content, opts.Chunk)
	}

	applyQueryRanking(&result, opts)

	return result
}

// applyQueryRanking populates Result.RankedSections from Result.Content when
// opts.Query is set. When opts.FilterByQuery is also true, Content is
// rewritten to the concatenated top-N sections (default top-3 when TopN<=0).
// No-op for an empty query — pure additive.
func applyQueryRanking(result *Result, opts Options) {
	if strings.TrimSpace(opts.Query) == "" {
		return
	}
	ranked := RankSections(result.Content, opts.Query, 1.5, 0.75, opts.TopN)
	if len(ranked) == 0 {
		return
	}
	result.RankedSections = ranked
	if !opts.FilterByQuery {
		return
	}
	limit := opts.TopN
	if limit <= 0 {
		limit = 3
	}
	if limit > len(ranked) {
		limit = len(ranked)
	}
	var sb strings.Builder
	for i := 0; i < limit; i++ {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		if ranked[i].Heading != "" {
			// Keep the heading prefix only when the section text itself doesn't
			// already start with it (chunkByHeading retains the heading line for
			// real sections; the prologue chunk has no heading).
			if !strings.HasPrefix(ranked[i].Text, ranked[i].Heading) {
				sb.WriteString(ranked[i].Heading)
				sb.WriteString("\n\n")
			}
		}
		sb.WriteString(ranked[i].Text)
	}
	result.Content = sb.String()
	result.Length = len(result.Content)
}

func processArticle(article readability.Article, targetURL string, start time.Time, parseStart time.Time, parseEnd time.Time) (result Result) {
	defer ensureProfile(&result)
	result = Result{URL: targetURL}

	convertStart := time.Now()
	markdown, err := convertHTMLToMarkdown(article.Content)
	convertEnd := time.Now()
	if err != nil {
		result.Error = err.Error()
		return result
	}

	markdown = CleanupMarkdown(markdown)

	result.Title = article.Title
	result.Content = markdown
	result.Byline = article.Byline
	result.Excerpt = article.Excerpt
	result.SiteName = article.SiteName
	result.Length = len(markdown)
	result.TimeMs = time.Since(start).Milliseconds()
	result.ParseTimeMs = parseEnd.Sub(parseStart).Milliseconds()
	result.ConvertTimeMs = convertEnd.Sub(convertStart).Milliseconds()

	result.HeadingCount = CountPattern(article.Content, `<h[1-6]`)
	htmlLinks := CountPattern(article.Content, `<a\s`)
	mdLinks := CountMarkdownLinks(markdown)
	if mdLinks > htmlLinks {
		result.LinkCount = mdLinks
	} else {
		result.LinkCount = htmlLinks
	}
	result.ParagraphCount = CountPattern(article.Content, `<p[\s>]`)

	result.Confidence = ComputeConfidence(result.Length, result.HeadingCount, result.ParagraphCount, 0, false)

	result.QualityInfo = ComputeQuality(markdown)
	result.Quality = result.QualityInfo.Score
	result.Fingerprint = SemanticFingerprint(markdown)
	result.Validation = ValidateExtraction(&result)
	result.ExtractionMethod = "readability"

	return result
}

func decompressBody(data []byte, encoding string) ([]byte, error) {
	switch encoding {
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer func() { _ = reader.Close() }()
		return io.ReadAll(reader)

	case "deflate":
		reader := flate.NewReader(bytes.NewReader(data))
		defer func() { _ = reader.Close() }()
		return io.ReadAll(reader)

	case "br":
		reader := brotli.NewReader(bytes.NewReader(data))
		return io.ReadAll(reader)

	case "zstd":
		reader, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)

	case "identity", "":
		return data, nil

	default:
		return data, nil
	}
}

// spawnBackgroundRefresh fires a conditional (or plain) GET in a goroutine to
// refresh a stale-while-revalidate cache entry. The foreground caller has
// already returned the stale body; this goroutine's job is to update the
// on-disk cache so the next call sees fresh data. All errors are silent —
// the only effect of a failure is that the entry stays stale for another
// SWR cycle. Uses a fresh http.Client to avoid sharing transports with the
// foreground request.
func spawnBackgroundRefresh(cache *DiskCache, cacheKey, targetURL string, cached *cachedResponse, userAgent string, opts Options) {
	go func() {
		req := newGETRequest(targetURL, userAgent, opts.RequestID, opts.SendRequestID)
		for k, v := range cached.ConditionalHeaders() {
			req.Header.Set(k, v)
		}
		client := &http.Client{Timeout: 30 * time.Second}
		if sharedC, err := getClientForOptions(opts); err == nil && sharedC != nil && sharedC.Transport != nil {
			client.Transport = sharedC.Transport
		}
		if opts.Transport != nil {
			client.Transport = opts.Transport
		}
		// Intentional silence (V1): the foreground caller already returned a
		// stale body; this goroutine's failures cannot be surfaced via
		// Result.Warnings because there's no Result to attach to. Failures
		// here merely keep the cache entry stale for another SWR cycle.
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusNotModified {
			_ = cache.TouchByKey(cacheKey)
			return
		}
		if resp.StatusCode == http.StatusOK {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return
			}
			_ = cache.Put(targetURL, req, http.StatusOK, resp.Header, bodyBytes)
		}
	}()
}

func ExtractFromHTML(html string, targetURL string) (string, error) {
	parsedURL, _ := url.Parse(targetURL)
	html = PreprocessHTMLWithURL(html, parsedURL)
	article, err := readability.FromReader(strings.NewReader(html), parsedURL)
	if err != nil {
		return "", err
	}

	markdown, err := convertHTMLToMarkdown(article.Content)
	if err != nil {
		return article.TextContent, nil
	}
	return markdown, nil
}
