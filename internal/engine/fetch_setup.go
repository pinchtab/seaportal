package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

var ErrBlockedByRobots = errors.New("blocked by robots.txt")

func buildFetchClient(opts Options, domain string, tracker *redirectTracker) (*http.Client, error) {
	timeout := opts.ClientTimeout
	if timeout <= 0 {
		timeout = DefaultClientTimeout
	}
	if opts.DomainTimeout != nil && domain != "" {
		if domainTimeout, ok := opts.DomainTimeout[domain]; ok && domainTimeout > 0 {
			timeout = domainTimeout
		}
	}

	checkRedirect := tracker.checkRedirect
	if opts.Security != nil {
		checkRedirect = opts.Security.redirectChecker(tracker)
	}

	sharedC, clientErr := getClientForOptions(opts)
	if clientErr != nil {
		return nil, clientErr
	}

	var client *http.Client
	if opts.NoPooling || (opts.DomainTimeout != nil && domain != "" && opts.DomainTimeout[domain] > 0) {
		client = &http.Client{Timeout: timeout, CheckRedirect: checkRedirect}
		if opts.Proxy != "" {
			client.Transport = sharedC.Transport
		}
	} else {
		client = &http.Client{
			Timeout:       timeout,
			Transport:     sharedC.Transport,
			CheckRedirect: checkRedirect,
		}
	}
	if opts.Security != nil && opts.Proxy == "" {
		client.Transport = &chromeTransport{security: opts.Security}
	}
	if opts.Transport != nil {
		client.Transport = opts.Transport
	}

	return client, nil
}

func resolveUserAgentFor(opts Options, domain string) string {
	userAgent := DefaultUserAgent
	if opts.UserAgent != "" {
		userAgent = ResolveUserAgent(opts.UserAgent)
	}
	if opts.DomainUserAgent != nil && domain != "" {
		if ua, ok := opts.DomainUserAgent[domain]; ok && ua != "" {
			userAgent = ua
		}
	}
	return userAgent
}

func robotsFetchOptions(opts Options) FetchBytesOptions {
	fo := FetchBytesOptions{Security: opts.Security}
	if opts.Proxy != "" {
		if client, err := getClientForOptions(opts); err == nil {
			c := *client
			c.Timeout = robotsFetchTimeout
			fo.Client = &c
		}
	}
	if opts.Transport != nil {
		fo.Client = &http.Client{Timeout: robotsFetchTimeout, Transport: opts.Transport}
	}
	return fo
}

func checkRobotsAllowed(ctx context.Context, opts Options, targetURL, domain, userAgent string, result *Result) bool {
	if !opts.RespectRobots || domain == "" {
		return true
	}
	cache := opts.CrawlDelayCache
	if cache == nil {
		cache = newCrawlDelayCacheWithFetch(robotsFetchOptions(opts))
	}
	parsed, perr := url.Parse(targetURL)
	if perr != nil {
		return true
	}
	scheme := parsed.Scheme
	if scheme == "" {
		scheme = "https"
	}
	host := parsed.Host
	if host == "" {
		host = domain
	}
	if cache.IsAllowed(ctx, host, userAgent, scheme, parsed.RequestURI()) {
		return true
	}
	result.setError(ErrBlockedByRobots)
	result.BlockedByRobots = true
	ensureProfile(result)
	result.Profile.Reasons = append(result.Profile.Reasons, "blocked-by-robots")
	return false
}

func applyCrawlDelay(ctx context.Context, opts Options, targetURL, domain, userAgent string) (warning string, err error) {
	if !opts.RespectCrawlDelay || domain == "" {
		return "", nil
	}
	crawlCache := opts.CrawlDelayCache
	if crawlCache == nil {
		crawlCache = newCrawlDelayCacheWithFetch(robotsFetchOptions(opts))
	}
	scheme := "https"
	host := domain
	if parsedURL, perr := url.Parse(targetURL); perr == nil {
		if parsedURL.Scheme != "" {
			scheme = parsedURL.Scheme
		}
		if parsedURL.Host != "" {
			host = parsedURL.Host
		}
	}
	delay := crawlCache.GetDelayWithScheme(ctx, host, userAgent, scheme)
	if delay <= 0 {
		return "", nil
	}
	if capped, clamped := clampCrawlDelay(delay); clamped {
		warning = fmt.Sprintf("robots.txt crawl-delay %s for %s clamped to %s", delay, host, maxCrawlDelay)
		delay = capped
	}
	return warning, sleepCtx(ctx, delay)
}

func applyRateLimit(ctx context.Context, opts Options, domain string) error {
	if opts.RateLimit <= 0 || domain == "" {
		return nil
	}
	limiter := opts.RateLimiter
	if limiter == nil {
		limiter = NewHostRateLimiter()
	}
	return limiter.Wait(ctx, domain, opts.RateLimit)
}
