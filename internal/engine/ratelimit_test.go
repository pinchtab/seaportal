package engine

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestHostRateLimiter_WaitsForInterval(t *testing.T) {
	l := NewHostRateLimiter()
	const host = "example.com"
	const interval = 200 * time.Millisecond

	l.Wait(host, interval) // first call records, no sleep

	start := time.Now()
	l.Wait(host, interval)
	elapsed := time.Since(start)

	if elapsed < 150*time.Millisecond {
		t.Fatalf("expected second call to sleep ~%v, slept %v", interval, elapsed)
	}
}

func TestHostRateLimiter_DifferentHostsIndependent(t *testing.T) {
	l := NewHostRateLimiter()
	const interval = 500 * time.Millisecond

	// Prime both hosts.
	l.Wait("a.example", interval)
	l.Wait("b.example", interval)

	// Now in parallel, hit a third unrelated host on each — clean state per host
	// means neither should block. Use distinct hosts to verify independence.
	var wg sync.WaitGroup
	start := time.Now()
	wg.Add(2)
	go func() {
		defer wg.Done()
		l.Wait("c.example", interval)
	}()
	go func() {
		defer wg.Done()
		l.Wait("d.example", interval)
	}()
	wg.Wait()
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Fatalf("different hosts should not block; elapsed %v", elapsed)
	}
}

func TestHostRateLimiter_ZeroIntervalNoOp(t *testing.T) {
	l := NewHostRateLimiter()
	l.Wait("example.com", 0)
	start := time.Now()
	l.Wait("example.com", 0)
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Fatalf("zero interval should not sleep; elapsed %v", elapsed)
	}
}

func TestHostRateLimiter_EmptyHostNoOp(t *testing.T) {
	l := NewHostRateLimiter()
	l.Wait("", 500*time.Millisecond)
	start := time.Now()
	l.Wait("", 500*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Fatalf("empty host should not sleep; elapsed %v", elapsed)
	}
}

func TestExtract_RateLimitAcrossSharedLimiter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><head><title>t</title></head><body><h1>hi</h1><p>body content here for extraction.</p></body></html>"))
	}))
	defer srv.Close()

	shared := NewHostRateLimiter()
	opts := Options{RateLimit: 150 * time.Millisecond, RateLimiter: shared}

	start := time.Now()
	for i := 0; i < 3; i++ {
		_ = FromURLWithOptions(srv.URL, opts)
	}
	elapsed := time.Since(start)

	if elapsed < 300*time.Millisecond {
		t.Fatalf("expected total elapsed >= 300ms (2 enforced waits between 3 requests), got %v", elapsed)
	}
}
