package engine

import (
	"context"
	"net/url"
	"sync"
	"time"
)

// defaultScrapeConcurrency bounds the fetch/extract worker pool by default.
const defaultScrapeConcurrency = 8

// poolConfig tunes the fetch/extract worker pool.
type poolConfig struct {
	// Concurrency is the number of workers; <= 0 uses defaultScrapeConcurrency.
	Concurrency int
	// MinInterval is the minimum spacing between requests to the same host. The
	// effective spacing is max(MinInterval, robots crawl-delay).
	MinInterval time.Duration
}

// fetchAll fetches and extracts every URL in urls concurrently via a bounded
// worker pool, returning PageObjects in the same order as urls. It honors:
//   - per-host rate limiting (shared HostRateLimiter) + robots crawl-delay,
//   - ctx cancellation and opts.Timeout (stops dispatching, returns partial),
//   - partial failures captured on the page's Error field (never aborts).
func fetchAll(ctx context.Context, urls []string, opts ScrapeOptions, cfg poolConfig) []PageObject {
	o := opts.normalized()
	respectRobots := o.RespectRobots != nil && *o.RespectRobots
	limiter := NewHostRateLimiter()
	robots := NewCrawlDelayCache()
	extractOpts := Options{UserAgent: o.UserAgent}

	return runFetchPool(ctx, urls, o.Timeout, cfg.Concurrency, func(ctx context.Context, u string) PageObject {
		if err := ctx.Err(); err != nil {
			return PageObject{URL: u, Error: err.Error()}
		}
		return fetchOne(ctx, u, extractOpts, limiter, robots, respectRobots, cfg.MinInterval)
	})
}

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

// fetchOne rate-limits, fetches, and extracts a single URL, mapping the extract
// Result onto a PageObject. Richer page assembly (meta, schema, perf, links) is
// ALP-006; here we carry the essentials plus any error.
func fetchOne(ctx context.Context, u string, extractOpts Options, limiter *HostRateLimiter, robots *CrawlDelayCache, respectRobots bool, minInterval time.Duration) PageObject {
	host, scheme := hostScheme(u)
	interval := minInterval
	if respectRobots && host != "" {
		if d := robots.GetDelayWithScheme(ctx, host, extractOpts.UserAgent, scheme); d > interval {
			interval = d
		}
	}
	if err := limiter.Wait(ctx, host, interval); err != nil {
		return PageObject{URL: u, Error: err.Error()}
	}

	// Bound the extract itself by the pool ctx too — a fetch dispatched just
	// before the deadline must not run past it.
	extractOpts.Context = ctx
	r := FromURLWithOptions(u, extractOpts)
	return PageObject{
		URL:      u,
		Title:    r.Title,
		Status:   r.StatusCode,
		Markdown: r.Content,
		Error:    r.Error,
	}
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
