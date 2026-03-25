// Package portal provides content extraction with SPA detection
package portal

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/andybalholm/brotli"
	"github.com/go-shiori/go-readability"
)

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

// DefaultUserAgent is the default User-Agent string used for HTTP requests.
// Must match a real browser exactly — Cloudflare blocks truncated/incomplete UAs.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

const DefaultAcceptEncoding = "gzip, deflate, br"

// newGETRequest creates a standard GET request with common headers.
// If userAgent is empty, DefaultUserAgent is used.
// If requestID is non-empty and sendRequestID is true, adds X-Request-ID header.
func newGETRequest(targetURL string, userAgent string, requestID string, sendRequestID bool) *http.Request {
	req, _ := http.NewRequest("GET", targetURL, nil)
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", DefaultAcceptEncoding)
	// Sec-* headers for Cloudflare bypass — modern browsers always send these
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

func FromURLWithOptions(targetURL string, opts Options) Result {
	start := time.Now()
	result := Result{URL: targetURL}

	domain := extractDomain(targetURL)

	timeout := 30 * time.Second
	if opts.DomainTimeout != nil && domain != "" {
		if domainTimeout, ok := opts.DomainTimeout[domain]; ok && domainTimeout > 0 {
			timeout = domainTimeout
		}
	}

	tracker := &redirectTracker{}

	var client *http.Client
	if opts.NoPooling || (opts.DomainTimeout != nil && domain != "" && opts.DomainTimeout[domain] > 0) {
		// Use a custom client if pooling is disabled OR if we have a domain-specific timeout
		client = &http.Client{Timeout: timeout, CheckRedirect: tracker.checkRedirect}
	} else {
		// Clone the shared client with our redirect tracker
		sharedC := getClient()
		client = &http.Client{
			Timeout:       sharedC.Timeout,
			Transport:     sharedC.Transport,
			CheckRedirect: tracker.checkRedirect,
		}
	}

	userAgent := DefaultUserAgent
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

	// HEAD pre-flight: detect permanent errors before doing a full GET
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
			// 405 Method Not Allowed: server doesn't support HEAD — fall through to GET
			// Any other status: proceed to GET normally
		}
		// On HEAD request error, proceed to GET anyway
	}

	if opts.RespectCrawlDelay && domain != "" {
		crawlCache := opts.CrawlDelayCache
		if crawlCache == nil {
			crawlCache = NewCrawlDelayCache()
		}
		scheme := "https"
		if parsedURL, err := url.Parse(targetURL); err == nil {
			scheme = parsedURL.Scheme
		}
		if delay := crawlCache.GetDelayWithScheme(domain, userAgent, scheme); delay > 0 {
			time.Sleep(delay)
		}
	}

	req := newGETRequest(targetURL, userAgent, opts.RequestID, opts.SendRequestID)

	var retryCount int
	var totalRetryWait time.Duration

	logRetry := func(event RetryEvent) {
		if opts.RetryLogger != nil {
			opts.RetryLogger(event)
		}
	}

	// Perform request with retry logic for transient errors (429, 502, 504)
	var resp *http.Response
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var err error
		resp, err = client.Do(req)
		if err != nil {
			if attempt < maxRetries && isRetryableError(err) {
				// Exponential backoff with jitter for connection errors
				backoff := time.Duration(1<<uint(attempt)) * time.Second
				if backoff > maxRetryWait {
					backoff = maxRetryWait
				}
				backoff = addJitter(backoff)
				// Check total retry timeout
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

		if (resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout) && attempt < maxRetries {
			// Exponential backoff with jitter for 502/504
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			if backoff > maxRetryWait {
				backoff = maxRetryWait
			}
			backoff = addJitter(backoff)
			// Check total retry timeout BEFORE closing body
			if totalRetryWait+backoff > totalRetryTimeout {
				logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, WaitTime: backoff, Outcome: "timeout"})
				break // Stop retrying, process the error response (body still open)
			}
			logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, WaitTime: backoff, Outcome: "retrying"})
			_ = resp.Body.Close()
			time.Sleep(backoff)
			retryCount++
			totalRetryWait += backoff
			req = newGETRequest(targetURL, userAgent, opts.RequestID, opts.SendRequestID)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			retryAfterHeader := resp.Header.Get("Retry-After")
			// If no Retry-After header provided, don't retry — proceed to blocked classification
			if retryAfterHeader == "" {
				logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, Outcome: "exhausted"})
				break
			}
			retryAfter, ok := parseRetryAfter(retryAfterHeader)
			if !ok || retryAfter > maxRetryWait {
				logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, WaitTime: retryAfter, Outcome: "exhausted"})
				break // Don't close body - we'll process this 429 response
			}
			// Check total retry timeout BEFORE closing body
			if totalRetryWait+retryAfter > totalRetryTimeout {
				logRetry(RetryEvent{Attempt: attempt + 1, StatusCode: resp.StatusCode, WaitTime: retryAfter, Outcome: "timeout"})
				break // Stop retrying, process the error response (body still open)
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

	// TTFB: time from start to headers received (client.Do has returned, headers available)
	ttfbMs := time.Since(start).Milliseconds()

	downloadStart := time.Now()
	bodyBytes, err := io.ReadAll(resp.Body)
	downloadMs := time.Since(downloadStart).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Decompress body based on Content-Encoding header
	// Go's http.Client only auto-decompresses when IT adds Accept-Encoding.
	// Since we set Accept-Encoding manually, we must decompress ourselves.
	contentEncoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
	if contentEncoding != "" {
		decompressed, decompErr := decompressBody(bodyBytes, contentEncoding)
		if decompErr != nil {
			// Decompression failed — try raw bytes as fallback.
			// Some servers advertise Content-Encoding but the HTTP/2 transport
			// already decompressed, or the body is empty/malformed.
			if len(bodyBytes) > 0 {
				// Check if body looks like HTML (already decompressed)
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

	html := string(bodyBytes)
	contentLength := int64(len(bodyBytes))
	fetchTimeMs := time.Since(start).Milliseconds()

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
			return result
		}
	}

	result = fromHTMLInternal(html, targetURL, start, opts)
	result.FetchTimeMs = fetchTimeMs
	result.TTFBMs = ttfbMs
	result.DownloadMs = downloadMs
	result.StatusCode = resp.StatusCode
	result.RetryCount = retryCount
	result.TotalRetryWait = totalRetryWait
	result.HeadPreflightStatus = headPreflightStatus
	result.ContentLength = contentLength
	result.RedirectCount = len(tracker.chain)
	result.RedirectChain = tracker.chain

	// FinalURL: the actual URL after all redirects (useful for canonicalization)
	if resp.Request != nil && resp.Request.URL != nil {
		result.FinalURL = resp.Request.URL.String()
	}

	populateResponseHeaders(&result, resp)

	result.TraceFormats, result.TraceCorrelation = computeTraceInfo(result)

	// CDN fingerprinting: identify CDN provider from header combinations
	result.CDNProvider, result.CDNSignals = fingerprintCDN(result)

	// Via header analysis for multi-tier CDN/proxy detection
	result.ViaHops = parseViaHeader(result.ResponseVia)
	result.ProxyLayers = len(result.ViaHops)

	result.RequestAcceptEncoding = DefaultAcceptEncoding
	result.RequestID = opts.RequestID

	// Soft-404 detection: check for error pages masquerading as 200 OK
	if resp.StatusCode == http.StatusOK {
		isSoft404, soft404Hints := detectSoft404(html, contentLength)
		result.IsSoft404 = isSoft404
		result.Soft404Hints = soft404Hints
	}

	// HTTP status code awareness: 401/403/429/502/503/504 are strong blocked signals
	// (502/504 only after retries exhausted - they reach here if still failing)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
		resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusBadGateway ||
		resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout {
		result.IsBlocked = true
		result.Profile.Class = PageBlocked
		result.Profile.Outcome = OutcomeNeedsBrowser
		result.Profile.Trustworthy = false
		var statusReason string
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			statusReason = "http-401-unauthorized"
		case http.StatusForbidden:
			statusReason = "http-403-forbidden"
		case http.StatusTooManyRequests:
			statusReason = "http-429-rate-limited"
		case http.StatusBadGateway:
			statusReason = "http-502-bad-gateway"
		case http.StatusServiceUnavailable:
			statusReason = "http-503-service-unavailable"
		case http.StatusGatewayTimeout:
			statusReason = "http-504-gateway-timeout"
		}
		result.Profile.Reasons = append(result.Profile.Reasons, statusReason)
	}

	return result
}

func FromHTML(html string, targetURL string) Result {
	start := time.Now()
	return fromHTMLInternal(html, targetURL, start, Options{})
}

func FromResponse(resp *http.Response, targetURL string, start time.Time) Result {
	result := Result{URL: targetURL}

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

func fromHTMLInternal(html string, targetURL string, start time.Time, opts Options) Result {
	spaSignals, isSPA := DetectSPA(html)
	isBlocked := DetectBlocked(html)

	// Preprocess HTML to preserve content that would otherwise be stripped
	html = PreprocessHTML(html)

	parsedURL, _ := url.Parse(targetURL)
	parseStart := time.Now()
	article, err := readability.FromReader(strings.NewReader(html), parsedURL)
	parseEnd := time.Now()
	if err != nil {
		return Result{URL: targetURL, Error: err.Error(), SPASignals: spaSignals, IsSPA: isSPA, IsBlocked: isBlocked}
	}

	result := processArticle(article, targetURL, start, parseStart, parseEnd)
	result.SPASignals = spaSignals
	result.IsSPA = isSPA
	result.IsBlocked = isBlocked

	result.Confidence = ComputeConfidence(result.Length, result.HeadingCount, result.ParagraphCount, len(result.SPASignals), result.IsBlocked)

	// Index page fallback: if readability gave poor results on a homepage/index page,
	// use direct headline extraction instead
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
	}

	// Text fallback: if readability and index fallback both gave poor results
	// but we have substantial HTML, try direct text extraction
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
		}
	}

	if result.Confidence < 30 {
		result.IsSPA = true
	}

	if opts.Dedupe && result.Content != "" {
		dedupeResult := Dedupe(result.Content)
		result.Content = dedupeResult.Content
		result.Length = len(result.Content)
		result.DedupeApplied = true
		result.DuplicatesRemoved = dedupeResult.DuplicatesFound
		result.DuplicateSignals = dedupeResult.DuplicateSignals
		result.OriginalBlockCount = dedupeResult.OriginalBlocks
		result.UniqueBlockCount = dedupeResult.UniqueBlocks

		// Recompute quality metrics after deduplication
		result.QualityInfo = ComputeQuality(result.Content)
		result.Quality = result.QualityInfo.Score
		result.Fingerprint = SemanticFingerprint(result.Content)
	}

	result.Profile = ClassifyPage(result)

	if opts.FailFast && result.IsSPA && result.Confidence < 30 {
		result.Error = fmt.Sprintf("SPA detected with low confidence (%d%%), signals: %v", result.Confidence, result.SPASignals)
	}

	return result
}

func processArticle(article readability.Article, targetURL string, start time.Time, parseStart time.Time, parseEnd time.Time) Result {
	result := Result{URL: targetURL}

	convertStart := time.Now()
	markdown, err := htmltomarkdown.ConvertString(article.Content)
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
	// Count links from both HTML and Markdown (for React/SPA pages)
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

	return result
}

// decompressBody decompresses response body based on Content-Encoding header.
// Supports gzip, deflate, and br (brotli).
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

	case "identity", "":
		return data, nil

	default:
		// Unknown encoding, return as-is (may be garbage, but don't error silently)
		return data, nil
	}
}

// ExtractFromHTML runs readability + markdown conversion on raw HTML string
func ExtractFromHTML(html string, targetURL string) (string, error) {
	html = PreprocessHTML(html)
	parsedURL, _ := url.Parse(targetURL)
	article, err := readability.FromReader(strings.NewReader(html), parsedURL)
	if err != nil {
		return "", err
	}

	markdown, err := htmltomarkdown.ConvertString(article.Content)
	if err != nil {
		return article.TextContent, nil
	}
	return markdown, nil
}
