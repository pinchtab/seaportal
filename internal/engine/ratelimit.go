package engine

import (
	"context"
	"sync"
	"time"
)

type HostRateLimiter struct {
	mu      sync.Mutex
	lastReq map[string]time.Time
}

func NewHostRateLimiter() *HostRateLimiter {
	return &HostRateLimiter{lastReq: make(map[string]time.Time)}
}

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
