package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
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

	reqCtx := opts.Context
	if reqCtx == nil {
		reqCtx = context.Background()
	}

	if opts.Security != nil {
		if err := opts.Security.ValidateURL(reqCtx, targetURL); err != nil {
			result.setError(err)
			result.SecurityBlock = err.Error()
			return result
		}
	}

	domain := extractDomain(targetURL)

	tracker := &redirectTracker{}

	client, clientErr := buildFetchClient(opts, domain, tracker)
	if clientErr != nil {
		result.setError(fmt.Errorf("invalid proxy URL: %w", clientErr))
		return result
	}

	userAgent := resolveUserAgentFor(opts, domain)

	if !checkRobotsAllowed(reqCtx, opts, targetURL, domain, userAgent, &result) {
		return result
	}

	if err := applyRateLimit(reqCtx, opts, domain); err != nil {
		result.setError(err)
		return result
	}

	req, err := http.NewRequestWithContext(reqCtx, "GET", targetURL, nil)
	if err != nil {
		result.setError(err)
		return result
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", DefaultAccept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", headOnlyByteCap-1))
	if opts.SendRequestID && opts.RequestID != "" {
		req.Header.Set("X-Request-ID", opts.RequestID)
	}

	resp, err := client.Do(req)
	if err != nil {
		result.setError(err)
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	result.Protocol = negotiatedProtocol(req, resp)

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, headOnlyByteCap))
	if err != nil {
		result.setError(err)
		return result
	}

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

	result.Content = ""
	result.Length = 0

	result.StatusCode = resp.StatusCode
	result.HeadPreflightStatus = resp.StatusCode
	result.ResponseContentType = respContentType
	if cl := resp.Header.Get("Content-Length"); cl != "" {
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
