package engine

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// negotiationRT returns a 404 (text/html) on the first call to trigger the
// markdown-negotiation HTML retry, then fails the retry fetch with a transport
// error so resp is left nil. Regression guard for the nil-resp panic in the
// deferred body close (extract.go): a transport error on the negotiation retry
// must surface as result.Error, not crash the process.
type negotiationRT struct{ calls int }

func (rt *negotiationRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.calls++
	if rt.calls == 1 {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(strings.NewReader("<html><body>not found</body></html>")),
			Request:    req,
		}, nil
	}
	return nil, fmt.Errorf("simulated dial failure on negotiation retry")
}

func TestNegotiationRetry_TransportError_NoPanic(t *testing.T) {
	rt := &negotiationRT{}
	opts := Options{
		Transport:         rt,
		MaxRetries:        1,
		MaxRetryWait:      time.Second,
		TotalRetryTimeout: 5 * time.Second,
	}

	// Must not panic; the failed retry should be reported as an error.
	result := FromURLWithOptions("https://example.com/page", opts)

	if rt.calls < 2 {
		t.Fatalf("expected the negotiation retry to fire a second fetch, calls=%d", rt.calls)
	}
	if result.Error == "" {
		t.Fatalf("expected result.Error from the failed negotiation retry, got none")
	}
}
