package engine

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

const backgroundRefreshTimeout = 30 * time.Second

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

type retryConfig struct {
	maxRetries        int
	maxRetryWait      time.Duration
	totalRetryTimeout time.Duration
	backoffBase       time.Duration
}

func resolveRetryConfig(opts Options, domain string) retryConfig {
	cfg := retryConfig{
		maxRetries:        opts.MaxRetries,
		maxRetryWait:      opts.MaxRetryWait,
		totalRetryTimeout: opts.TotalRetryTimeout,
		backoffBase:       opts.RetryBackoffBase,
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
		cfg.maxRetryWait = DefaultMaxRetryWait
	}
	if cfg.totalRetryTimeout == 0 {
		cfg.totalRetryTimeout = DefaultTotalRetryTimeout
	}
	if cfg.backoffBase <= 0 {
		cfg.backoffBase = DefaultRetryBackoffBase
	}
	return cfg
}

func fetchDocument(targetURL string, opts Options, start time.Time, result *Result) (st fetchState, ok bool) {
	reqCtx := opts.Context
	if reqCtx == nil {
		reqCtx = context.Background()
	}
	st.reqCtx = reqCtx
	st.tracker = &redirectTracker{}

	if strings.HasPrefix(targetURL, "data:") {
		mime, body, derr := parseDataURL(targetURL)
		if derr != nil {
			result.setError(derr)
			return st, false
		}
		switch mime {
		case "text/html", "text/plain":
			*result = fromHTMLInternal(string(body), targetURL, start, opts)
		default:
			result.setError(errors.New("data: URL mime not supported: " + mime))
		}
		return st, false
	}

	if opts.Security != nil {
		if err := opts.Security.ValidateURL(reqCtx, targetURL); err != nil {
			result.setError(err)
			result.SecurityBlock = err.Error()
			return st, false
		}
	}

	domain := extractDomain(targetURL)

	client, clientErr := buildFetchClient(opts, domain, st.tracker)
	if clientErr != nil {
		result.setError(fmt.Errorf("invalid proxy URL: %w", clientErr))
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
					result.setError(fmt.Errorf("skipped binary content: %s", contentType))
					return st, false
				}
			}

			if opts.HeadPreflight {
				if st.headPreflightStatus == http.StatusNotFound || st.headPreflightStatus == http.StatusGone {
					result.HeadPreflightStatus = st.headPreflightStatus
					result.setError(fmt.Errorf("HEAD preflight returned %d", st.headPreflightStatus))
					return st, false
				}
			}
		}
	}

	if !checkRobotsAllowed(reqCtx, opts, targetURL, domain, st.userAgent, result) {
		return st, false
	}

	crawlWarning, crawlErr := applyCrawlDelay(reqCtx, opts, targetURL, domain, st.userAgent)
	if crawlWarning != "" {
		st.preWarnings = append(st.preWarnings, crawlWarning)
	}
	if crawlErr != nil {
		result.setError(crawlErr)
		return st, false
	}

	if err := applyRateLimit(reqCtx, opts, domain); err != nil {
		result.setError(err)
		return st, false
	}

	req := newGETRequest(targetURL, st.userAgent, opts.RequestID, opts.SendRequestID).WithContext(reqCtx)

	var cache *DiskCache
	if opts.CacheDir != "" {
		c, cacheErr := NewDiskCache(opts.CacheDir, opts.CacheTTL)
		if cacheErr != nil {
			fmt.Fprintf(os.Stderr, "warning: cache init failed: %v\n", cacheErr)
		} else {
			cache = c
		}
	}

	lookup := cacheLookupStage(cache, opts, targetURL, st.userAgent, req, result)
	resp := lookup.resp

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
		result.setError(err)
		if errors.Is(err, ErrResponseTooLarge) {
			result.SecurityBlock = err.Error()
		}
		return st, false
	}

	if cache != nil && !result.CacheHit && !result.CacheRevalidated && !result.CacheStale && resp.StatusCode == http.StatusOK {
		if putErr := cache.Put(targetURL, req, resp.StatusCode, resp.Header, bodyBytes); putErr != nil {
			st.preWarnings = append(st.preWarnings, fmt.Sprintf("cache write: %v", putErr))
		}
	}

	st.bodyBytes, st.preWarnings, st.respContentType = decompressAndRestoreCharsetStage(bodyBytes, resp, opts, result, st.preWarnings)
	if result.Error != "" {
		return st, false
	}

	ctLower := strings.ToLower(st.respContentType)
	isPDF := strings.Contains(ctLower, "application/pdf")
	if isBinaryContentType(st.respContentType) || (isPDF && opts.NoPDF) {
		result.setError(fmt.Errorf("skipped binary content: %s", st.respContentType))
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

func renegotiateHTML(targetURL string, opts Options, st *fetchState, result *Result) bool {
	_ = st.resp.Body.Close()
	req := newGETRequest(targetURL, st.userAgent, opts.RequestID, opts.SendRequestID).WithContext(st.reqCtx)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	resp, err := st.client.Do(req)
	if err != nil {
		result.setError(err)
		return false
	}
	st.resp = resp

	bodyBytes, err := limitedReadAll(resp.Body, st.maxBody, ErrResponseTooLarge)
	if err != nil {
		result.setError(err)
		if errors.Is(err, ErrResponseTooLarge) {
			result.SecurityBlock = err.Error()
		}
		return false
	}
	st.bodyBytes, st.preWarnings, st.respContentType = decompressAndRestoreCharsetStage(bodyBytes, resp, opts, result, st.preWarnings)
	return result.Error == ""
}

type cacheLookup struct {
	resp        *http.Response
	hit         bool
	pendingMeta *cachedResponse
	pendingBody []byte
	pendingKey  string
}

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

type retryWaitOutcome int

const (
	retryProceed retryWaitOutcome = iota
	retryBudgetExceeded
	retryCanceled
)

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
	backoffBase := retryCfg.backoffBase

	ctx := req.Context()

	logRetry := func(event RetryEvent) {
		if opts.RetryLogger != nil {
			opts.RetryLogger(event)
		}
	}

	expBackoff := func(attempt int) time.Duration {
		backoff := time.Duration(1<<uint(attempt)) * backoffBase
		if backoff > maxRetryWait {
			backoff = maxRetryWait
		}
		return addJitter(backoff)
	}

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
			result.setError(werr)
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
						result.setError(err)
					}
					return nil
				}
				continue
			}
			logRetry(RetryEvent{Attempt: attempt + 1, Error: err, Outcome: "exhausted"})
			result.setError(err)
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

func decompressAndRestoreCharsetStage(bodyBytes []byte, resp *http.Response, opts Options, result *Result, preWarnings []string) ([]byte, []string, string) {
	contentEncoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
	if contentEncoding != "" {
		var maxDecomp int64
		if opts.Security != nil {
			maxDecomp = opts.Security.MaxDecompressedBytes
		}
		decompressed, decompErr := decompressBodyLimited(bodyBytes, contentEncoding, maxDecomp)
		if errors.Is(decompErr, ErrDecompressTooLarge) {
			result.setError(decompErr)
			result.SecurityBlock = decompErr.Error()
			return nil, preWarnings, ""
		}
		if decompErr != nil {
			if len(bodyBytes) > 0 {
				trimmed := bytes.TrimSpace(bodyBytes)
				if len(trimmed) > 0 && (trimmed[0] == '<' || trimmed[0] == '{') {
				} else {
					result.setError(fmt.Errorf("decompression error (%s): %w", contentEncoding, decompErr))
					return nil, preWarnings, ""
				}
			} else {
				result.setError(fmt.Errorf("decompression error (%s): %w", contentEncoding, decompErr))
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

func spawnBackgroundRefresh(cache *DiskCache, cacheKey, targetURL string, cached *cachedResponse, userAgent string, opts Options) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), backgroundRefreshTimeout)
		defer cancel()
		req := newGETRequest(targetURL, userAgent, opts.RequestID, opts.SendRequestID).WithContext(ctx)
		for k, v := range cached.ConditionalHeaders() {
			req.Header.Set(k, v)
		}
		client := &http.Client{Timeout: backgroundRefreshTimeout}
		if sharedC, err := getClientForOptions(opts); err == nil && sharedC != nil && sharedC.Transport != nil {
			client.Transport = sharedC.Transport
		}
		if opts.Transport != nil {
			client.Transport = opts.Transport
		}
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
