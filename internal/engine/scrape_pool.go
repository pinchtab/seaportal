package engine

import (
	"context"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultScrapeConcurrency = 8

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

func canonicalHost(host, scheme string) string {
	host = strings.ToLower(host)
	h, port, err := net.SplitHostPort(host)
	if err != nil {
		return host
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
