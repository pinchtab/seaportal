package engine

import (
	"context"
	"net/url"
	"sync"
	"time"
)

// defaultScrapeConcurrency bounds the fetch/extract worker pool by default.
const defaultScrapeConcurrency = 8

// runFetchPool maps do over urls with a bounded worker pool, preserving order.
// It applies timeout to ctx, stops dispatching once ctx is cancelled, and fills
// any URL not reached with a cancellation error — so callers always get one
// PageObject per input URL (partial results on cancellation).
func runFetchPool(ctx context.Context, urls []string, timeout time.Duration, concurrency int, do func(context.Context, string) PageObject) []PageObject {
	results := make([]PageObject, len(urls))
	if len(urls) == 0 {
		return results
	}
	if concurrency <= 0 {
		concurrency = defaultScrapeConcurrency
	}
	if concurrency > len(urls) {
		concurrency = len(urls)
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for idx := range jobs {
			results[idx] = do(ctx, urls[idx])
		}
	}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker()
	}

dispatch:
	for i := range urls {
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()

	for i := range results {
		if results[i].URL == "" {
			results[i] = PageObject{URL: urls[i], Error: context.Canceled.Error()}
		}
	}
	return results
}

func hostScheme(raw string) (host, scheme string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "https"
	}
	scheme = u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return u.Host, scheme
}
