package leakcheck

import (
	"runtime"
	"testing"
	"time"
)

const tolerance = 2

const waitWindow = 200 * time.Millisecond

const pollInterval = 10 * time.Millisecond

func CheckLeak(tb testing.TB) {
	tb.Helper()
	runtime.Gosched()
	before := runtime.NumGoroutine()

	tb.Cleanup(func() {
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
		tb.Errorf("goroutine leak: started with %d, ended with %d (delta %d > tolerance %d)\n--- goroutine dump ---\n%s",
			before, after, after-before, tolerance, buf[:n])
	})
}
