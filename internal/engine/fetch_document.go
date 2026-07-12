package engine

// fetch_document.go — the fetch/transport/policy stage of FromURLWithOptions.
//
// fetchDocument owns everything that happens before content-type dispatch:
// the data: URL shortcut, SSRF gate, client assembly, HEAD preflight, robots
// gate, crawl-delay, rate limit, cache lookup, retried fetch, 304 replay,
// body read + cache write, decompress/charset restore, and the binary gate.
// Its output is a fetchState that the per-content-type pipelines
// (extract_pdf.go, extract_raw.go, extract_markdown.go, fromHTMLInternal)
// and the shared finalizeTransport tail consume.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// retryBackoffBase is the unit of exponential retry backoff (hop N waits
// 2^N × base, capped by maxRetryWait). Production is 1s; tests shrink it to a
// few milliseconds so retry-path coverage runs without real-time sleeps.
var retryBackoffBase = time.Second

// fetchState carries the artifacts of a completed fetch stage into the
// per-content-type pipelines: the response and decoded body, the transport
// pieces needed for a renegotiation refetch (client, ctx, UA, body cap), and
// the telemetry that finalizeTransport stamps onto the Result.
type fetchState struct {
	resp            *http.Response
	bodyBytes       []byte
	respContentType string

	client    *http.Client
	reqCtx    context.Context
	userAgent string
	maxBody   int64

	tracker             *redirectTracker
	preWarnings         []string
	ttfbMs              int64
	downloadMs          int64
	retryCount          int
	totalRetryWait      time.Duration
	headPreflightStatus int
}

// retryConfig is the resolved retry budget for one fetch.
type retryConfig struct {
	maxRetries        int
	maxRetryWait      time.Duration
	totalRetryTimeout time.Duration
}

// resolveRetryConfig resolves opts-level retry settings plus any per-domain
// DomainRetryConfig override, applying the 60s wait / 120s total defaults.
func resolveRetryConfig(opts Options, domain string) retryConfig {
	cfg := retryConfig{
		maxRetries:        opts.MaxRetries,
		maxRetryWait:      opts.MaxRetryWait,
		totalRetryTimeout: opts.TotalRetryTimeout,
	}
	if opts.DomainRetryConfig != nil && domain != "" {
		if domainCfg, ok := opts.DomainRetryConfig[domain]; ok {
			if domainCfg.MaxRetries > 0 {
				cfg.maxRetries = domainCfg.MaxRetries
			}
			if domainCfg.MaxRetryWait > 0 {
				cfg.maxRetryWait = domainCfg.MaxRetryWait
			}
		}
	}
	if cfg.maxRetryWait == 0 {
		cfg.maxRetryWait = 60 * time.Second
	}
	if cfg.totalRetryTimeout == 0 {
		cfg.totalRetryTimeout = 120 * time.Second
	}
	return cfg
}

// fetchDocument runs the transport/policy stage for targetURL. It returns
// ok=false with result populated when the fetch ended early — a policy block,
// transport error, binary body, or the data: URL shortcut (which runs the
// full HTML pipeline itself). The caller owns closing st.resp's body.
func fetchDocument(targetURL string, opts Options, start time.Time, result *Result) (st fetchState, ok bool) {
	// Cancellation context for the whole fetch: bounds the request and makes
	// crawl-delay / retry backoff waits interruptible (ALP-043). Defaults to
	// Background so existing callers that don't set opts.Context are unchanged.
	reqCtx := opts.Context
	if reqCtx == nil {
		reqCtx = context.Background()
	}
	st.reqCtx = reqCtx
	st.tracker = &redirectTracker{}

	// data: URL short-circuit (RFC 2397). Bypasses the network entirely:
	// decode inline body and feed it straight into the HTML pipeline. Scope
	// is strictly text/html + text/plain; other media types (binary/image)
	// error cleanly so we don't pipe non-text bytes through readability.
	if strings.HasPrefix(targetURL, "data:") {
		mime, body, derr := parseDataURL(targetURL)
		if derr != nil {
			result.Error = derr.Error()
			return st, false
		}
		switch mime {
		case "text/html", "text/plain":
			*result = fromHTMLInternal(string(body), targetURL, start, opts)
		default:
			result.Error = "data: URL mime not supported: " + mime
		}
		return st, false
	}

	// Pre-fetch security gate: validate scheme + domain + resolved IP before any
	// socket is opened. Placed after the data: short-circuit (data: never hits
	// the network) so the policy only governs real network fetches. The dial
	// Control hook re-checks the resolved IP at connect to close DNS rebinding.
	if opts.Security != nil {
		if err := opts.Security.ValidateURL(reqCtx, targetURL); err != nil {
			result.Error = err.Error()
			result.SecurityBlock = err.Error()
			return st, false
		}
	}

	domain := extractDomain(targetURL)

	client, clientErr := buildFetchClient(opts, domain, st.tracker)
	if clientErr != nil {
		result.Error = "invalid proxy URL: " + clientErr.Error()
		return st, false
	}
	st.client = client

	st.userAgent = resolveUserAgentFor(opts, domain)

	retryCfg := resolveRetryConfig(opts, domain)

	if opts.HeadPreflight || opts.ContentTypePreflight {
		headReq, _ := http.NewRequestWithContext(reqCtx, "HEAD", targetURL, nil)
		headReq.Header.Set("User-Agent", st.userAgent)
		headResp, headErr := client.Do(headReq)
		if headErr == nil {
			st.headPreflightStatus = headResp.StatusCode
			contentType := headResp.Header.Get("Content-Type")
			_ = headResp.Body.Close()

			if opts.ContentTypePreflight && st.headPreflightStatus == http.StatusOK {
				if isBinaryContentType(contentType) {
					result.HeadPreflightStatus = st.headPreflightStatus
					result.Error = fmt.Sprintf("skipped binary content: %s", contentType)
					return st, false
				}
			}

			if opts.HeadPreflight {
				if st.headPreflightStatus == http.StatusNotFound || st.headPreflightStatus == http.StatusGone {
					result.HeadPreflightStatus = st.headPreflightStatus
					result.Error = fmt.Sprintf("HEAD preflight returned %d", st.headPreflightStatus)
					return st, false
				}
			}
		}
	}

	if !checkRobotsAllowed(opts, targetURL, domain, st.userAgent, result) {
		return st, false
	}

	if err := applyCrawlDelay(reqCtx, opts, targetURL, domain, st.userAgent); err != nil {
		result.Error = err.Error()
		return st, false
	}

	if err := applyRateLimit(reqCtx, opts, domain); err != nil {
		result.Error = err.Error()
		return st, false
	}

	req := newGETRequest(targetURL, st.userAgent, opts.RequestID, opts.SendRequestID).WithContext(reqCtx)

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

	// Cache lookup and revalidation header setup.
	lookup := cacheLookupStage(cache, opts, targetURL, st.userAgent, req, result)
	resp := lookup.resp

	// Fetch with retry stage.
	if !lookup.hit {
		resp = fetchWithRetryStage(req, targetURL, opts, &st, retryCfg, result)
		if result.Error != "" && resp == nil {
			result.RetryCount = st.retryCount
			result.TotalRetryWait = st.totalRetryWait
			return st, false
		}
	}
	st.resp = resp

	result.Protocol = negotiatedProtocol(req, resp)

	st.ttfbMs = time.Since(start).Milliseconds()

	// 304 Not Modified for a stale cache entry: replay the cached body and
	// refresh FetchedAt so subsequent calls see it as fresh.
	if lookup.pendingMeta != nil && resp.StatusCode == http.StatusNotModified {
		_ = resp.Body.Close()
		resp = lookup.pendingMeta.toHTTPResponse(lookup.pendingBody, req)
		st.resp = resp
		if cache != nil && lookup.pendingKey != "" {
			if touchErr := cache.TouchByKey(lookup.pendingKey); touchErr != nil {
				st.preWarnings = append(st.preWarnings, fmt.Sprintf("cache touch: %v", touchErr))
			}
		}
		result.CacheRevalidated = true
	}

	downloadStart := time.Now()
	if opts.Security != nil {
		st.maxBody = opts.Security.MaxResponseBytes
	}
	bodyBytes, err := limitedReadAll(resp.Body, st.maxBody, ErrResponseTooLarge)
	st.downloadMs = time.Since(downloadStart).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		if errors.Is(err, ErrResponseTooLarge) {
			result.SecurityBlock = err.Error()
		}
		return st, false
	}

	// Persist successful 200 OK responses to the on-disk cache. Cache the raw
	// (still-encoded) wire bytes so replay flows through the same decompress
	// path as a live fetch. Errors are non-fatal: log and continue. Skip when
	// we just replayed a 304 (CacheRevalidated) — TouchByKey already handled it.
	if cache != nil && !result.CacheHit && !result.CacheRevalidated && !result.CacheStale && resp.StatusCode == http.StatusOK {
		if putErr := cache.Put(targetURL, req, resp.StatusCode, resp.Header, bodyBytes); putErr != nil {
			st.preWarnings = append(st.preWarnings, fmt.Sprintf("cache write: %v", putErr))
		}
	}

	// Decompress and charset recovery stage.
	st.bodyBytes, st.preWarnings, st.respContentType = decompressAndRestoreCharsetStage(bodyBytes, resp, opts, result, st.preWarnings)
	if result.Error != "" {
		return st, false
	}

	// Binary gate on the DEFAULT path (ALP-038): image/audio/video/archive/
	// octet-stream bodies must never reach readability/markdown — binary
	// bytes parsed as HTML stall for minutes. The same guard previously ran
	// only in the opt-in HEAD preflight. PDFs are exempt (extracted by the
	// PDF pipeline) unless the caller opted out via --no-pdf.
	ctLower := strings.ToLower(st.respContentType)
	isPDF := strings.Contains(ctLower, "application/pdf")
	if isBinaryContentType(st.respContentType) || (isPDF && opts.NoPDF) {
		result.Error = fmt.Sprintf("skipped binary content: %s", st.respContentType)
		result.StatusCode = resp.StatusCode
		result.ContentLength = int64(len(st.bodyBytes))
		result.ResponseContentType = st.respContentType
		result.TimeMs = time.Since(start).Milliseconds()
		result.FetchTimeMs = time.Since(start).Milliseconds()
		result.TTFBMs = st.ttfbMs
		result.DownloadMs = st.downloadMs
		return st, false
	}

	return st, true
}

// renegotiateHTML retries the fetch with a plain-HTML Accept header after a
// 404/406 suggested the server rejected `Accept: text/markdown` outright
// (Next.js responds 404, spec-compliant servers 406). Replaces st.resp /
// st.bodyBytes / st.respContentType in place; returns false with result
// populated when the refetch failed.
func renegotiateHTML(targetURL string, opts Options, st *fetchState, result *Result) bool {
	_ = st.resp.Body.Close()
	req := newGETRequest(targetURL, st.userAgent, opts.RequestID, opts.SendRequestID).WithContext(st.reqCtx)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	resp, err := st.client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return false
	}
	st.resp = resp

	bodyBytes, err := limitedReadAll(resp.Body, st.maxBody, ErrResponseTooLarge)
	if err != nil {
		result.Error = err.Error()
		if errors.Is(err, ErrResponseTooLarge) {
			result.SecurityBlock = err.Error()
		}
		return false
	}
	st.bodyBytes, st.preWarnings, st.respContentType = decompressAndRestoreCharsetStage(bodyBytes, resp, opts, result, st.preWarnings)
	return result.Error == ""
}

// cacheLookup is the outcome of cacheLookupStage: a replayable response for
// fresh/SWR hits, or the pending metadata for a synchronous revalidation.
type cacheLookup struct {
	resp        *http.Response
	hit         bool
	pendingMeta *cachedResponse
	pendingBody []byte
	pendingKey  string
}

// cacheLookupStage handles cache read, SWR band, and revalidation header setup.
func cacheLookupStage(cache *DiskCache, opts Options, targetURL string, userAgent string, req *http.Request, result *Result) cacheLookup {
	var lookup cacheLookup

	if cache != nil && !opts.NoCache {
		meta, body, fresh, swrStale, beyondTolerance := cache.GetStaleWithTolerance(targetURL, req, opts.CacheStaleTolerance)
		if fresh {
			lookup.resp = meta.toHTTPResponse(body, req)
			result.CacheHit = true
			lookup.hit = true
		} else if swrStale {
			lookup.resp = meta.toHTTPResponse(body, req)
			result.CacheStale = true
			lookup.hit = true
			bgKey := cache.cacheKey(targetURL, req)
			spawnBackgroundRefresh(cache, bgKey, targetURL, meta, userAgent, opts)
		} else if beyondTolerance {
			lookup.pendingKey = cache.cacheKey(targetURL, req)
			lookup.pendingMeta = meta
			lookup.pendingBody = body
			for k, v := range meta.ConditionalHeaders() {
				req.Header.Set(k, v)
			}
		}
	}

	return lookup
}

// retryWaitOutcome reports how the shared retry tail resolved: proceed with
// another attempt, stop because the totalRetryTimeout budget would be
// exceeded, or stop because the context was cancelled mid-wait.
type retryWaitOutcome int

const (
	retryProceed retryWaitOutcome = iota
	retryBudgetExceeded
	retryCanceled
)

// fetchWithRetryStage executes the fetch with exponential backoff retry
// logic, recording retry counters into st.
func fetchWithRetryStage(req *http.Request, targetURL string, opts Options, st *fetchState, retryCfg retryConfig, result *Result) *http.Response {
	var resp *http.Response
	var retryCount int
	var totalRetryWait time.Duration
	defer func() {
		st.retryCount = retryCount
		st.totalRetryWait = totalRetryWait
	}()

	client := st.client
	userAgent := st.userAgent
	maxRetries := retryCfg.maxRetries
	maxRetryWait := retryCfg.maxRetryWait
	totalRetryTimeout := retryCfg.totalRetryTimeout

	// The request's context bounds the whole retry loop: backoff sleeps below
	// select on it so the deadline / SIGINT can interrupt a wait, and rebuilt
	// requests inherit it so client.Do stays cancellable across attempts.
	ctx := req.Context()

	logRetry := func(event RetryEvent) {
		if opts.RetryLogger != nil {
			opts.RetryLogger(event)
		}
	}

	// maxRetryWait is resolved by the caller (opts.MaxRetryWait plus any
	// per-domain DomainRetryConfig override, defaulted to 60s); the stage just
	// clamps to it (ALP-047).
	expBackoff := func(attempt int) time.Duration {
		backoff := time.Duration(1<<uint(attempt)) * retryBackoffBase
		if backoff > maxRetryWait {
			backoff = maxRetryWait
		}
		return addJitter(backoff)
	}

	// waitAndRetry is the shared tail of every retry branch: budget check,
	// logging, body close, ctx-aware wait, counter increments, and request
	// rebuild. event carries the branch's template (Attempt plus StatusCode
	// or Error); closeBody is nil when the branch has no response to release.
	// The body is only closed on the retrying path — on budget exhaustion the
	// response is handed back to the caller unread.
	waitAndRetry := func(wait time.Duration, event RetryEvent, closeBody *http.Response) retryWaitOutcome {
		event.WaitTime = wait
		if totalRetryWait+wait > totalRetryTimeout {
			event.Outcome = "timeout"
			logRetry(event)
			return retryBudgetExceeded
		}
		event.Outcome = "retrying"
		logRetry(event)
		if closeBody != nil {
			_ = closeBody.Body.Close()
		}
		if werr := sleepCtx(ctx, wait); werr != nil {
			event.Error = werr
			event.Outcome = "canceled"
			logRetry(event)
			result.Error = werr.Error()
			return retryCanceled
		}
		retryCount++
		totalRetryWait += wait
		req = newGETRequest(targetURL, userAgent, opts.RequestID, opts.SendRequestID).WithContext(ctx)
		return retryProceed
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		var err error
		resp, err = client.Do(req)
		if err != nil {
			if attempt < maxRetries && isRetryableError(err) {
				outcome := waitAndRetry(expBackoff(attempt), RetryEvent{Attempt: attempt + 1, Error: err}, nil)
				if outcome != retryProceed {
					if result.Error == "" {
						result.Error = err.Error()
					}
					return nil
				}
				continue
			}
			logRetry(RetryEvent{Attempt: attempt + 1, Error: err, Outcome: "exhausted"})
			result.Error = err.Error()
			return nil
		}

		if (resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout || (resp.StatusCode == http.StatusServiceUnavailable && resp.Header.Get("Retry-After") == "")) && attempt < maxRetries {
			outcome := waitAndRetry(expBackoff(attempt), RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode}, resp)
			if outcome == retryCanceled {
				return nil
			}
			if outcome == retryBudgetExceeded {
				break
			}
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
			outcome := waitAndRetry(retryAfter, RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode}, resp)
			if outcome == retryCanceled {
				return nil
			}
			if outcome == retryBudgetExceeded {
				break
			}
			continue
		}

		if resp.StatusCode == http.StatusOK {
			logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, Outcome: "success"})
		}
		break
	}

	return resp
}

// decompressAndRestoreCharsetStage handles decompression and charset detection.
func decompressAndRestoreCharsetStage(bodyBytes []byte, resp *http.Response, opts Options, result *Result, preWarnings []string) ([]byte, []string, string) {
	contentEncoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
	if contentEncoding != "" {
		var maxDecomp int64
		if opts.Security != nil {
			maxDecomp = opts.Security.MaxDecompressedBytes
		}
		decompressed, decompErr := decompressBodyLimited(bodyBytes, contentEncoding, maxDecomp)
		if errors.Is(decompErr, ErrDecompressTooLarge) {
			result.Error = decompErr.Error()
			result.SecurityBlock = decompErr.Error()
			return nil, preWarnings, ""
		}
		if decompErr != nil {
			if len(bodyBytes) > 0 {
				trimmed := bytes.TrimSpace(bodyBytes)
				if len(trimmed) > 0 && (trimmed[0] == '<' || trimmed[0] == '{') {
				} else {
					result.Error = fmt.Sprintf("decompression error (%s): %v", contentEncoding, decompErr)
					return nil, preWarnings, ""
				}
			} else {
				result.Error = fmt.Sprintf("decompression error (%s): %v", contentEncoding, decompErr)
				return nil, preWarnings, ""
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
			preWarnings = append(preWarnings, fmt.Sprintf("charset decode: declared charset %q not recognised, falling back to raw bytes", declared))
		}
	}

	return bodyBytes, preWarnings, respContentType
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
