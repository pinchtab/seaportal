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

func getClient() *http.Client {
	return getUTLSClient()
}

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

func parseRetryAfter(header string) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}

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

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return !dnsErr.IsNotFound
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	if errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return false
}
