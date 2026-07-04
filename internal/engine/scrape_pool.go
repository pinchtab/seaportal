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
	results := make([]PageObject, len(urls))
	if len(urls) == 0 {
		return results
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = defaultScrapeConcurrency
	}
	if concurrency > len(urls) {
		concurrency = len(urls)
	}

	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}

	respectRobots := o.RespectRobots != nil && *o.RespectRobots
	limiter := NewHostRateLimiter()
	robots := NewCrawlDelayCache()
	extractOpts := Options{UserAgent: o.UserAgent}

	jobs := make(chan int)
	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for idx := range jobs {
			u := urls[idx]
			if err := ctx.Err(); err != nil {
				results[idx] = PageObject{URL: u, Error: err.Error()}
				continue
			}
			results[idx] = fetchOne(u, extractOpts, limiter, robots, respectRobots, cfg.MinInterval)
		}
	}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker()
	}

	// Dispatch, stopping early if ctx is cancelled.
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

	// Any index never dispatched (ctx cancelled mid-dispatch) → partial result.
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
func fetchOne(u string, extractOpts Options, limiter *HostRateLimiter, robots *CrawlDelayCache, respectRobots bool, minInterval time.Duration) PageObject {
	host, scheme := hostScheme(u)
	interval := minInterval
	if respectRobots && host != "" {
		if d := robots.GetDelayWithScheme(host, extractOpts.UserAgent, scheme); d > interval {
			interval = d
		}
	}
	limiter.Wait(host, interval)

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
