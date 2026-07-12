package engine

import (
	"context"
	"net"
	"net/url"
	"strings"
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

// canonicalHost lowercases host and strips the port when it is the scheme's
// default (80 for http, 443 for https), so "example.com:80" and "example.com"
// compare equal. A sitemap or link that omits the default port must still be
// recognised as same-host as a base URL that includes it (and vice versa).
func canonicalHost(host, scheme string) string {
	host = strings.ToLower(host)
	h, port, err := net.SplitHostPort(host)
	if err != nil {
		return host // no port present
	}
	switch {
	case scheme == "http" && port == "80":
		return h
	case scheme == "https" && port == "443":
		return h
	default:
		return host
	}
}

// sameHost reports whether rawURL is on the same host as the base (identified
// by baseHost/baseScheme), treating default ports as equivalent to no port.
func sameHost(rawURL, baseHost, baseScheme string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	uScheme := u.Scheme
	if uScheme == "" {
		uScheme = baseScheme
	}
	return canonicalHost(u.Host, uScheme) == canonicalHost(baseHost, baseScheme)
}
