// Package leakcheck provides a test helper that fails a Go test when it
// finishes with more live goroutines than it started with. The intent is
// to catch background goroutine leaks (e.g. forgotten SWR refreshers,
// abandoned worker pools, httptest servers that weren't Close()d) at
// test time rather than as slow memory drift in production.
//
// # Usage
//
//	func TestSomething(t *testing.T) {
//	    leakcheck.CheckLeak(t)
//	    // ... rest of test
//	}
//
// CheckLeak snapshots runtime.NumGoroutine() on entry and registers a
// t.Cleanup hook that polls (with backoff) for up to waitWindow,
// allowing in-flight goroutines to wind down before asserting.
//
// # Tolerance
//
// We allow a small jitter band (tolerance=2) above the entry count.
// Why not zero? The Go runtime and the testing framework themselves spin
// up short-lived goroutines (GC workers, finalisers, parallel-test
// scaffolding) that can ebb and flow across a single test. A delta of 2
// has been empirically sufficient to absorb that noise while still
// catching real leaks (which typically grow by 1 per fetch, i.e. tens or
// hundreds over a stress test). A wider band would start to mask bugs.
//
// # t.Parallel caveat
//
// Do NOT call CheckLeak inside a subtest that uses t.Parallel(). Parallel
// siblings spawn goroutines that pollute both the snapshot and the
// cleanup count — false positives and false negatives both become likely.
// Use CheckLeak only in serial tests (or in the parent test, before any
// t.Run with t.Parallel()).
//
// # SWR background-refresh caveat
//
// Tests that exercise the stale-while-revalidate cache path
// (extract.go::spawnBackgroundRefresh) fire a background HTTP request
// that outlives the foreground response. The 200ms wait window normally
// absorbs this, but on a slow or contended machine it may flake. If you
// see flakes in an SWR-exercising test, the right fix is to make the
// test wait deterministically (e.g. inject a done channel) — not to
// widen the tolerance here.
package leakcheck

import (
	"runtime"
	"testing"
	"time"
)

// tolerance is the allowed delta in live goroutines between entry and
// cleanup. See package doc for rationale.
const tolerance = 2

// waitWindow caps how long we'll poll for transient goroutines to exit
// before declaring a leak.
const waitWindow = 200 * time.Millisecond

// pollInterval is the gap between polls within waitWindow.
const pollInterval = 10 * time.Millisecond

// CheckLeak snapshots the current goroutine count and registers a
// t.Cleanup hook that fails the test (via t.Errorf, with a full
// goroutine stack dump) if the count grew by more than `tolerance`
// goroutines and stayed grown for the entire waitWindow.
//
// Pass any testing.TB-compatible value; the helper uses Errorf rather
// than Fatalf so the test still records other failures from its body.
func CheckLeak(t testing.TB) {
	t.Helper()
	runtime.Gosched()
	before := runtime.NumGoroutine()

	t.Cleanup(func() {
		deadline := time.Now().Add(waitWindow)
		var after int
		for {
			runtime.Gosched()
			after = runtime.NumGoroutine()
			if after-before <= tolerance {
				return
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(pollInterval)
		}
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		t.Errorf("goroutine leak: started with %d, ended with %d (delta %d > tolerance %d)\n--- goroutine dump ---\n%s",
			before, after, after-before, tolerance, buf[:n])
	})
}
