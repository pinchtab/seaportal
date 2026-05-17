package leakcheck

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeTB is a minimal testing.TB shim that captures Errorf calls and
// records Cleanup hooks so the test driving CheckLeak can run them
// explicitly. We need this because CheckLeak signals failure via
// t.Errorf, and the meta-test "DetectsLeak" *expects* that failure —
// it must observe it without failing the surrounding real *testing.T.
//
// Only the methods CheckLeak actually calls are implemented; the rest
// are stubs that panic, which forces us to notice if CheckLeak grows a
// new dependency on testing.TB.
type fakeTB struct {
	testing.TB
	mu       sync.Mutex
	cleanups []func()
	errs     []string
	failed   atomic.Bool
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Cleanup(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanups = append(f.cleanups, fn)
}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
	f.failed.Store(true)
}

func (f *fakeTB) runCleanups() {
	f.mu.Lock()
	cs := append([]func(){}, f.cleanups...)
	f.mu.Unlock()
	// Cleanups run in LIFO order, matching testing.T.
	for i := len(cs) - 1; i >= 0; i-- {
		cs[i]()
	}
}

func (f *fakeTB) errorJoined() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.errs, "\n")
}

// TestLeakcheck_PassesOnClean ensures CheckLeak does not flag a test
// that spawns no goroutines of its own.
func TestLeakcheck_PassesOnClean(t *testing.T) {
	fake := &fakeTB{}
	CheckLeak(fake)
	fake.runCleanups()
	if fake.failed.Load() {
		t.Fatalf("CheckLeak reported a leak on a clean test: %s", fake.errorJoined())
	}
}

// TestLeakcheck_DetectsLeak spawns goroutines that outlive the test
// body and asserts CheckLeak flags them. The goroutines are gated on a
// `done` channel that this test's own t.Cleanup closes — so they
// terminate before the process moves on, keeping the suite leak-free
// even though the helper saw them at cleanup time.
func TestLeakcheck_DetectsLeak(t *testing.T) {
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })

	fake := &fakeTB{}
	CheckLeak(fake)

	// Spawn enough goroutines to comfortably exceed `tolerance`.
	const n = tolerance + 5
	started := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			started <- struct{}{}
			<-done
		}()
	}
	// Wait for all goroutines to actually be live before triggering
	// cleanup; otherwise the snapshot may be taken before they start.
	for i := 0; i < n; i++ {
		<-started
	}

	fake.runCleanups()

	if !fake.failed.Load() {
		t.Fatalf("CheckLeak did not detect %d leaked goroutines", n)
	}
	if !strings.Contains(fake.errorJoined(), "goroutine leak") {
		t.Fatalf("error message missing 'goroutine leak': %s", fake.errorJoined())
	}
}

// TestLeakcheck_AbsorbsTransients spawns a goroutine that exits well
// within the wait window. The poll-with-backoff in CheckLeak must wait
// for it to finish rather than flagging a false positive.
func TestLeakcheck_AbsorbsTransients(t *testing.T) {
	fake := &fakeTB{}
	CheckLeak(fake)

	// Spawn a handful of goroutines that each exit after ~50ms — well
	// under waitWindow (200ms) but well over a single Gosched.
	const n = tolerance + 3
	for i := 0; i < n; i++ {
		go func() {
			time.Sleep(50 * time.Millisecond)
		}()
	}

	fake.runCleanups()

	if fake.failed.Load() {
		t.Fatalf("CheckLeak flagged transient goroutines that should have been absorbed: %s", fake.errorJoined())
	}
}
