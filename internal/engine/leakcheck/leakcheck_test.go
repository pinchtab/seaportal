package leakcheck

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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
	for i := len(cs) - 1; i >= 0; i-- {
		cs[i]()
	}
}

func (f *fakeTB) errorJoined() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.errs, "\n")
}

func TestLeakcheck_PassesOnClean(t *testing.T) {
	fake := &fakeTB{}
	CheckLeak(fake)
	fake.runCleanups()
	if fake.failed.Load() {
		t.Fatalf("CheckLeak reported a leak on a clean test: %s", fake.errorJoined())
	}
}

func TestLeakcheck_DetectsLeak(t *testing.T) {
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })

	fake := &fakeTB{}
	CheckLeak(fake)

	const n = tolerance + 5
	started := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			started <- struct{}{}
			<-done
		}()
	}
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

func TestLeakcheck_AbsorbsTransients(t *testing.T) {
	fake := &fakeTB{}
	CheckLeak(fake)

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
