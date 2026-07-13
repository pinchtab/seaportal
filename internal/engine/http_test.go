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

func TestIsRetryableError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},

		{"io.EOF wrapped", fmt.Errorf("read response body: %w", io.EOF), true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"unexpected EOF inside OpError", &net.OpError{Op: "read", Net: "tcp", Err: io.ErrUnexpectedEOF}, true},

		{"ECONNRESET via OpError", &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, true},
		{"ECONNREFUSED via OpError", &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, true},
		{"EPIPE via OpError", &net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("write", syscall.EPIPE)}, true},

		{"DNS not found (NXDOMAIN) is permanent", &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "nope.invalid", IsNotFound: true}}, false},
		{"DNS temporary failure", &net.DNSError{Err: "server misbehaving", Name: "flaky.example", IsTemporary: true}, true},
		{"DNS timeout", &net.DNSError{Err: "i/o timeout", Name: "slow.example", IsTimeout: true}, true},

		{"os.ErrDeadlineExceeded (net.Error timeout)", os.ErrDeadlineExceeded, true},
		{"timeout wrapped in OpError", &net.OpError{Op: "read", Net: "tcp", Err: &timeoutError{}}, true},

		{"context.Canceled", context.Canceled, false},
		{"context.Canceled wrapped like url.Error", fmt.Errorf("Get %q: %w", "https://example.com", context.Canceled), false},
		{"context.DeadlineExceeded classifies as timeout", context.DeadlineExceeded, true},

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

type timeoutError struct{}

func (*timeoutError) Error() string   { return "synthetic timeout" }
func (*timeoutError) Timeout() bool   { return true }
func (*timeoutError) Temporary() bool { return true }

var _ net.Error = (*timeoutError)(nil)

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

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
