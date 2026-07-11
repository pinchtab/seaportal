package engine

import (
	"context"
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

// Wait blocks until host's next request slot, spacing requests to the same
// host by minInterval. The slot is reserved under the lock and the wait
// happens outside it, so a long crawl-delay on one host never blocks other
// hosts, and concurrent callers for the same host each reserve successive
// slots (ALP-048; the previous lock-held sleep serialized every host behind
// one wait). Returns ctx's error if the context fires before the slot; the
// reservation is kept — cancellation means the scrape is tearing down.
// No-op when minInterval <= 0 or host is empty.
func (l *HostRateLimiter) Wait(ctx context.Context, host string, minInterval time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if minInterval <= 0 || host == "" {
		return ctx.Err()
	}
	now := time.Now()
	l.mu.Lock()
	next := now
	if last, ok := l.lastReq[host]; ok {
		if slot := last.Add(minInterval); slot.After(now) {
			next = slot
		}
	}
	l.lastReq[host] = next
	l.mu.Unlock()
	return sleepCtx(ctx, time.Until(next))
}
