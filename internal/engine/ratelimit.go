package engine

import (
	"sync"
	"time"
)

// HostRateLimiter enforces a minimum interval between requests to the same
// host. Mutex-guarded map; thread-safe. Callers wanting cross-call
// throttling must share one instance via Options.RateLimiter.
type HostRateLimiter struct {
	mu      sync.Mutex
	lastReq map[string]time.Time
}

// NewHostRateLimiter returns a fresh limiter with an empty last-request map.
func NewHostRateLimiter() *HostRateLimiter {
	return &HostRateLimiter{lastReq: make(map[string]time.Time)}
}

// Wait sleeps if the current time is within minInterval of the last
// recorded request for host. No-op when minInterval <= 0 or host is empty.
func (l *HostRateLimiter) Wait(host string, minInterval time.Duration) {
	if minInterval <= 0 || host == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if last, ok := l.lastReq[host]; ok {
		elapsed := time.Since(last)
		if elapsed < minInterval {
			time.Sleep(minInterval - elapsed)
		}
	}
	l.lastReq[host] = time.Now()
}
