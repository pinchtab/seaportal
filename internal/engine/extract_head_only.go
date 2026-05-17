package engine

// extract_head_only.go — cheap metadata-only fetch path.
//
// fetchHeadOnly issues a single GET with Range: bytes=0-16383 + Accept-Encoding:
// identity to grab just enough body for `<head>` parsing. The 16 KB cap is
// enforced with io.LimitReader regardless of whether the server honoured Range
// (200 responses with full body still get truncated). Identity encoding avoids
// the "partial gzip stream" failure mode where a 16 KB slice of a longer gzip
// stream can't be decompressed.
//
// Only the metadata extractors run: ExtractLDJSON, applyLDJSONMetadata,
// ExtractMetadata, applyMetadata, PickCanonical, plus a small <title> regex.
// Readability/preprocess/sanitize/dedupe/links/images/citations are all
// skipped — they're irrelevant for triage. Result.HeadOnly is set so callers
// can disambiguate from a normal fetch.
//
// Distinct from Options.HeadPreflight which is a true HTTP HEAD (zero body).

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const headOnlyByteCap = 16384

var headOnlyTitleRE = regexp.MustCompile(`(?is)<title[^>]*>([^<]*)</title>`)

func fetchHeadOnly(targetURL string, opts Options) (result Result) {
	defer ensureProfile(&result)
	start := time.Now()
	result = Result{URL: targetURL, HeadOnly: true}

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

	if opts.RateLimit > 0 && domain != "" {
		limiter := opts.RateLimiter
		if limiter == nil {
			limiter = NewHostRateLimiter()
		}
		limiter.Wait(domain, opts.RateLimit)
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", DefaultAccept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// Force identity: a partial gzip stream returned via Range is undecodable.
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", headOnlyByteCap-1))
	if opts.SendRequestID && opts.RequestID != "" {
		req.Header.Set("X-Request-ID", opts.RequestID)
	}

	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	result.Protocol = negotiatedProtocol(req, resp)

	// Cap read at 16 KB even when the server ignores Range.
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, headOnlyByteCap))
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Defensive: server may still send Content-Encoding despite our identity ask.
	// Full-stream decompression of a 16 KB slice will usually fail; on failure we
	// just continue with the raw bytes — head metadata regexes tolerate junk.
	contentEncoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
	if contentEncoding != "" && contentEncoding != "identity" {
		if decompressed, decompErr := decompressBody(bodyBytes, contentEncoding); decompErr == nil {
			bodyBytes = decompressed
		}
	}

	respContentType := resp.Header.Get("Content-Type")
	if isCharsetSniffableContentType(respContentType) {
		if decoded, cs, ok := sniffAndDecode(bodyBytes, respContentType); ok {
			bodyBytes = decoded
			result.Charset = cs
		} else if declared := detectCharset(bodyBytes, respContentType); declared != "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("charset decode: declared charset %q not recognised, falling back to raw bytes", declared))
		}
	}

	html := string(bodyBytes)

	ldBlocks := ExtractLDJSON(html)
	applyLDJSONMetadata(&result, ldBlocks)
	applyMetadata(&result, ExtractMetadata(html))

	if result.Title == "" {
		if m := headOnlyTitleRE.FindStringSubmatch(html); len(m) > 1 {
			result.Title = strings.TrimSpace(m[1])
		}
	}

	if pick := PickCanonical(targetURL, html); pick != "" && pick != targetURL {
		result.CanonicalURL = pick
	}

	// Explicitly zeroed: even if metadata Author code path injected a byline
	// prefix into Content, head-only must not surface body content.
	result.Content = ""
	result.Length = 0

	result.StatusCode = resp.StatusCode
	result.HeadPreflightStatus = resp.StatusCode
	result.ResponseContentType = respContentType
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		// best-effort, ignore parse errors
		var n int64
		_, _ = fmt.Sscanf(cl, "%d", &n)
		result.ContentLength = n
	}
	result.RedirectCount = tracker.count
	result.RedirectChain = tracker.chain
	if resp.Request != nil && resp.Request.URL != nil {
		result.FinalURL = resp.Request.URL.String()
	}
	result.TimeMs = time.Since(start).Milliseconds()
	result.FetchTimeMs = result.TimeMs
	result.RequestID = opts.RequestID

	return result
}
