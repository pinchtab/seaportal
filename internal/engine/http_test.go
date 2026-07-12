package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestIsRetryableError locks in the typed retryability contract (T04): only
// errors that are provably transient — matched via errors.Is / errors.As, no
// substring sniffing — are retried. The former substring fallback made any
// error containing "EOF" retryable and treated "no such host" (NXDOMAIN, a
// permanent answer) as transient, burning the whole retry budget on typo'd
// domains.
//
// HTTP status-driven retries (429/502/503/504, Retry-After) are NOT decided
// by isRetryableError — fetchWithRetryStage branches on resp.StatusCode
// before this function is ever consulted, so there are no status cases here.
func TestIsRetryableError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},

		// Truncated reads.
		{"io.EOF wrapped", fmt.Errorf("read response body: %w", io.EOF), true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"unexpected EOF inside OpError", &net.OpError{Op: "read", Net: "tcp", Err: io.ErrUnexpectedEOF}, true},

		// Connection errno failures as the transport actually delivers them:
		// syscall errno wrapped in *os.SyscallError wrapped in *net.OpError.
		{"ECONNRESET via OpError", &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, true},
		{"ECONNREFUSED via OpError", &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, true},
		{"EPIPE via OpError", &net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("write", syscall.EPIPE)}, true},

		// DNS: NXDOMAIN is permanent — never retry; other DNS failures are
		// transient. *net.DNSError also implements net.Error, so these cases
		// pin that the DNS branch wins over the generic timeout branch.
		{"DNS not found (NXDOMAIN) is permanent", &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "nope.invalid", IsNotFound: true}}, false},
		{"DNS temporary failure", &net.DNSError{Err: "server misbehaving", Name: "flaky.example", IsTemporary: true}, true},
		{"DNS timeout", &net.DNSError{Err: "i/o timeout", Name: "slow.example", IsTimeout: true}, true},

		// Generic net.Error timeouts (read/write deadline, handshake timeout).
		{"os.ErrDeadlineExceeded (net.Error timeout)", os.ErrDeadlineExceeded, true},
		{"timeout wrapped in OpError", &net.OpError{Op: "read", Net: "tcp", Err: &timeoutError{}}, true},

		// Context cancellation must not be retried — the caller gave up.
		{"context.Canceled", context.Canceled, false},
		{"context.Canceled wrapped like url.Error", fmt.Errorf("Get %q: %w", "https://example.com", context.Canceled), false},
		// context.DeadlineExceeded implements net.Error with Timeout()==true,
		// so it classifies as retryable; the retry loop's ctx-aware backoff
		// wait (sleepCtx) then aborts immediately, so no time is wasted.
		{"context.DeadlineExceeded classifies as timeout", context.DeadlineExceeded, true},

		// Regression: the substring fallback is gone. String-only lookalikes
		// with no typed cause in the chain must NOT be retried.
		{"string-only EOF", errors.New("premature EOF while parsing frame"), false},
		{"string-only no such host", errors.New("lookup nope.invalid: no such host"), false},
		{"string-only connection reset", errors.New("read tcp 1.2.3.4:443: connection reset by peer"), false},
		{"string-only i/o timeout", errors.New("i/o timeout"), false},
		{"generic error", errors.New("boom"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableError(tc.err); got != tc.want {
				t.Errorf("isRetryableError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// timeoutError is a minimal net.Error with Timeout()==true and no errno or
// DNS identity — exercising the generic timeout branch in isolation.
type timeoutError struct{}

func (*timeoutError) Error() string   { return "synthetic timeout" }
func (*timeoutError) Timeout() bool   { return true }
func (*timeoutError) Temporary() bool { return true }

var _ net.Error = (*timeoutError)(nil)

// TestIsRetryableError_RetryLoopIntegration drives the real fetch retry loop
// with a permanent DNS failure injected at the transport and asserts the
// budget is NOT spent: NXDOMAIN must fail fast with zero retries.
func TestIsRetryableError_RetryLoopIntegration(t *testing.T) {
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{
			Err: "no such host", Name: req.URL.Hostname(), IsNotFound: true,
		}}
	})
	opts := Options{
		Transport:         rt,
		MaxRetries:        3,
		MaxRetryWait:      time.Second,
		TotalRetryTimeout: 5 * time.Second,
	}
	res := FromURLWithOptions("https://definitely-not-a-real-host.invalid/", opts)
	if res.Error == "" {
		t.Fatal("expected a DNS error surfaced on Result.Error")
	}
	if res.RetryCount != 0 {
		t.Errorf("RetryCount = %d, want 0 (NXDOMAIN is permanent, no retry)", res.RetryCount)
	}
}

// roundTripperFunc adapts a function to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
