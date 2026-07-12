package engine

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
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

	// Try seconds first (most common), then fall back to HTTP-date.
	if seconds, err := time.ParseDuration(header + "s"); err == nil {
		return seconds, true
	}

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
	jitterFactor := 0.75 + rand.Float64()*0.5
	result := time.Duration(float64(d) * jitterFactor)
	if result < time.Millisecond {
		result = time.Millisecond
	}
	return result
}

// isRetryableError reports whether err is a transient network failure worth
// another attempt. Matching is strictly typed (errors.Is / errors.As) with no
// error-string sniffing: a bare "EOF" substring matches unrelated errors, and
// "no such host" (NXDOMAIN — a permanent answer) would burn the whole retry
// budget on every typo'd domain (T04).
//
// Retryable: dropped/refused/broken connections (ECONNRESET, ECONNREFUSED,
// EPIPE), truncated reads (io.EOF, io.ErrUnexpectedEOF), any net.Error
// timeout, and non-NXDOMAIN DNS failures. Not retryable: DNS "no such host",
// context cancellation, and everything else. HTTP status retries (429/502/
// 503/504) are decided in fetchWithRetryStage, not here.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// DNS gets first say: IsNotFound (NXDOMAIN) is authoritative and permanent,
	// while other DNS failures (resolver timeout, SERVFAIL, flaky upstream) are
	// worth a retry. Checked before the generic timeout branch because
	// *net.DNSError also implements net.Error.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return !dnsErr.IsNotFound
	}

	// Truncated response: the peer (or a middlebox) cut the connection mid-body.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// Connection-level errno failures surface wrapped in *net.OpError /
	// *os.SyscallError; errors.Is unwraps the chain.
	if errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}

	// Any transport timeout (dial, TLS handshake, response header, deadline).
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return false
}
