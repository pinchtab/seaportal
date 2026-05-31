package engine

import (
	"testing"
	"time"
)

// TestMain shrinks the exponential retry backoff base for the whole package so
// retry-path tests exercise the real logic without real-time sleeps (the
// production default is 1s; 5ms keeps relative growth assertions valid while
// cutting seconds off the suite). Retry-After-driven waits are unaffected —
// they sleep the header value, not this base.
func TestMain(m *testing.M) {
	retryBackoffBase = 5 * time.Millisecond
	m.Run()
}
