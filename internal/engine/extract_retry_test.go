package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const retryTestBody = `<html><body><article><h1>Success</h1><p>Content here.</p></article></body></html>`

func TestRetry_503WithRetryAfter(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(retryTestBody))
	}))
	defer server.Close()

	opts := Options{
		MaxRetries:        2,
		MaxRetryWait:      5 * time.Second,
		TotalRetryTimeout: 10 * time.Second,
	}
	result := FromURLWithOptions(server.URL, opts)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", result.RetryCount)
	}
	if result.TotalRetryWait < time.Second {
		t.Errorf("TotalRetryWait = %v, want >= 1s", result.TotalRetryWait)
	}
}

func TestRetry_503WithoutRetryAfter(t *testing.T) {
	attempts := 0
	var waitTimes []time.Duration
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(retryTestBody))
	}))
	defer server.Close()

	opts := Options{
		MaxRetries:        3,
		MaxRetryWait:      5 * time.Second,
		TotalRetryTimeout: 30 * time.Second,
		RetryLogger: func(event RetryEvent) {
			if event.Outcome == "retrying" && event.StatusCode == http.StatusServiceUnavailable {
				waitTimes = append(waitTimes, event.WaitTime)
			}
		},
	}
	result := FromURLWithOptions(server.URL, opts)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", result.RetryCount)
	}
	if len(waitTimes) != 2 {
		t.Fatalf("expected 2 retry wait events, got %d", len(waitTimes))
	}
	// Exponential backoff: second wait should be larger than first (allowing jitter +/-25%).
	// First base = 1s, second base = 2s. Minimum second = 1.5s, max first = 1.25s.
	if waitTimes[1] <= waitTimes[0] {
		t.Errorf("expected exponential growth, got waits %v then %v", waitTimes[0], waitTimes[1])
	}
}

func TestRetry_503ExhaustedReportsError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	opts := Options{
		MaxRetries:        2,
		MaxRetryWait:      2 * time.Second,
		TotalRetryTimeout: 10 * time.Second,
	}
	result := FromURLWithOptions(server.URL, opts)

	if result.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", result.RetryCount)
	}
	if result.Error == "" && !result.IsBlocked && result.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected error or 503 status surfaced, got status=%d error=%q", result.StatusCode, result.Error)
	}
	if attempts != 3 { // initial + 2 retries
		t.Errorf("server saw %d requests, want 3", attempts)
	}
}

func TestRetry_CLIFlagsRespectMaxRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	opts := Options{
		MaxRetries:        1,
		MaxRetryWait:      2 * time.Second,
		TotalRetryTimeout: 10 * time.Second,
	}
	result := FromURLWithOptions(server.URL, opts)

	if result.RetryCount > 1 {
		t.Errorf("RetryCount = %d, want <= 1", result.RetryCount)
	}
	if attempts > 2 { // initial + at most 1 retry
		t.Errorf("server saw %d requests, want <= 2", attempts)
	}
}
