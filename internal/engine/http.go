// Package portal provides content extraction with SPA detection
package engine

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

type redirectTracker struct {
	chain []string
	// count is the number of redirect hops attempted (i.e. checkRedirect
	// calls observed). Increments BEFORE the 10-hop cap check, so a loop
	// killed by the cap reports count = 10 — matching what an operator
	// would expect: "ten redirects were attempted against the cap of ten."
	// `chain` only records the URL-from-the-previous-request and is gated
	// on the cap, so it has fewer entries than `count` when the cap fires.
	// Use `count` for `Result.RedirectCount`; `chain` is the trail of URLs.
	count int
}

func (rt *redirectTracker) checkRedirect(req *http.Request, via []*http.Request) error {
	rt.count++
	if len(via) >= 10 {
		return http.ErrUseLastResponse
	}
	if len(via) > 0 {
		rt.chain = append(rt.chain, via[len(via)-1].URL.String())
	}
	return nil
}

// getClient returns the shared HTTP client with Chrome TLS fingerprint impersonation.
// Uses utls to bypass Cloudflare and other bot detection that fingerprint Go's TLS stack.
func getClient() *http.Client {
	return getUTLSClient()
}

// getClientForOptions returns the shared no-proxy client when opts.Proxy is
// empty, or a fresh proxy-aware client otherwise. An invalid proxy URL is
// reported back so callers can fail fast with a helpful Result.Error.
func getClientForOptions(opts Options) (*http.Client, error) {
	if opts.Proxy == "" {
		return getClient(), nil
	}
	parsed, err := url.Parse(opts.Proxy)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("proxy URL must include scheme and host (got %q)", opts.Proxy)
	}
	return getUTLSClientWithProxy(parsed), nil
}

// parseRetryAfter parses the Retry-After header value.
// Returns (duration, true) on success, or (0, false) if unparseable.
// A duration of 0 with ok=true means "retry immediately" (Retry-After: 0).
// Supports both delay-seconds (e.g., "120") and HTTP-date formats.
func parseRetryAfter(header string) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}

	// Try parsing as seconds first (most common)
	if seconds, err := time.ParseDuration(header + "s"); err == nil {
		return seconds, true
	}

	// Try parsing as HTTP-date (RFC1123)
	if t, err := time.Parse(time.RFC1123, header); err == nil {
		wait := time.Until(t)
		if wait < 0 {
			wait = 0
		}
		return wait, true
	}

	return 0, false
}

// addJitter adds randomized jitter (±25%) to a duration to prevent thundering herd.
// Returns the duration with jitter applied; minimum result is 1ms.
func addJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	// ±25% jitter: multiply by 0.75 to 1.25
	jitterFactor := 0.75 + rand.Float64()*0.5
	result := time.Duration(float64(d) * jitterFactor)
	if result < time.Millisecond {
		result = time.Millisecond
	}
	return result
}

// isRetryableError checks if an error is a transient network error that should be retried.
// Covers connection reset, connection refused, timeouts, and temporary DNS failures.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.Temporary() {
		return true
	}

	errStr := err.Error()
	retryablePatterns := []string{
		"connection reset",
		"connection refused",
		"no such host", // Temporary DNS failure
		"i/o timeout",
		"EOF",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}
