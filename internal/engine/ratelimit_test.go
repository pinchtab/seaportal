package engine

import (
	"context"
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

	_ = l.Wait(context.Background(), host, interval) // first call records, no sleep

	start := time.Now()
	_ = l.Wait(context.Background(), host, interval)
	elapsed := time.Since(start)

	if elapsed < 150*time.Millisecond {
		t.Fatalf("expected second call to sleep ~%v, slept %v", interval, elapsed)
	}
}

func TestHostRateLimiter_DifferentHostsIndependent(t *testing.T) {
	l := NewHostRateLimiter()
	const interval = 500 * time.Millisecond

	// Prime both hosts.
	_ = l.Wait(context.Background(), "a.example", interval)
	_ = l.Wait(context.Background(), "b.example", interval)

	// Now in parallel, hit a third unrelated host on each — clean state per host
	// means neither should block. Use distinct hosts to verify independence.
	var wg sync.WaitGroup
	start := time.Now()
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = l.Wait(context.Background(), "c.example", interval)
	}()
	go func() {
		defer wg.Done()
		_ = l.Wait(context.Background(), "d.example", interval)
	}()
	wg.Wait()
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Fatalf("different hosts should not block; elapsed %v", elapsed)
	}
}

func TestHostRateLimiter_ZeroIntervalNoOp(t *testing.T) {
	l := NewHostRateLimiter()
	_ = l.Wait(context.Background(), "example.com", 0)
	start := time.Now()
	_ = l.Wait(context.Background(), "example.com", 0)
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Fatalf("zero interval should not sleep; elapsed %v", elapsed)
	}
}

func TestHostRateLimiter_EmptyHostNoOp(t *testing.T) {
	l := NewHostRateLimiter()
	_ = l.Wait(context.Background(), "", 500*time.Millisecond)
	start := time.Now()
	_ = l.Wait(context.Background(), "", 500*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Fatalf("empty host should not sleep; elapsed %v", elapsed)
	}
}

// regression: ALP-048(a) — ctx cancellation must interrupt a pending
// crawl-delay wait instead of blocking for the full interval.
func TestHostRateLimiter_CtxCancelInterruptsWait(t *testing.T) {
	l := NewHostRateLimiter()
	const host = "slow.example"
	_ = l.Wait(context.Background(), host, time.Millisecond) // prime

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := l.Wait(ctx, host, 10*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a context error from an interrupted wait, got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("cancelled wait took %v; must return promptly, not sleep 10s", elapsed)
	}
}

// regression: ALP-048(b) — the wait must happen outside the limiter mutex so
// one host's long crawl-delay does not serialize other hosts.
func TestHostRateLimiter_SlowHostDoesNotBlockOthers(t *testing.T) {
	l := NewHostRateLimiter()
	_ = l.Wait(context.Background(), "slow.example", time.Millisecond) // prime
	_ = l.Wait(context.Background(), "fast.example", time.Millisecond) // prime

	slowStarted := make(chan struct{})
	slowDone := make(chan struct{})
	go func() {
		close(slowStarted)
		_ = l.Wait(context.Background(), "slow.example", 3*time.Second)
		close(slowDone)
	}()

	<-slowStarted
	time.Sleep(50 * time.Millisecond) // let the slow waiter enter its wait

	start := time.Now()
	if err := l.Wait(context.Background(), "fast.example", 100*time.Millisecond); err != nil {
		t.Fatalf("fast host wait errored: %v", err)
	}
	elapsed := time.Since(start)

	select {
	case <-slowDone:
		t.Fatal("slow host wait finished too early; test setup invalid")
	default:
	}
	if elapsed > time.Second {
		t.Fatalf("fast host waited %v behind slow host's 3s crawl-delay; hosts must not serialize", elapsed)
	}
}

// Concurrent same-host callers must still space out by minInterval each
// (slot reservation), not collapse onto the same wake-up time.
func TestHostRateLimiter_SameHostConcurrentCallersSpaced(t *testing.T) {
	l := NewHostRateLimiter()
	const host = "shared.example"
	const interval = 150 * time.Millisecond
	_ = l.Wait(context.Background(), host, interval) // prime

	var wg sync.WaitGroup
	start := time.Now()
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_ = l.Wait(context.Background(), host, interval)
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	// Two waiters after a fresh prime must reserve prime+1 and prime+2 slots:
	// the later one returns no earlier than ~2×interval.
	if elapsed < 2*interval-50*time.Millisecond {
		t.Fatalf("concurrent same-host waiters finished in %v; expected ~%v (two spaced slots)", elapsed, 2*interval)
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
