package engine

// fetch_setup.go — fetch-path setup shared by FromURLWithOptions (via
// fetchDocument) and fetchHeadOnly: client construction, user-agent
// resolution, and the politeness gates (robots.txt, crawl-delay, rate limit).
// Both entry points previously carried byte-identical copies of this logic;
// any fix here now applies to both.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// buildFetchClient assembles the http.Client for a fetch: per-domain timeout,
// redirect tracking (security-checked when a policy is set), the shared pooled
// transport vs a throwaway client, the security dial guard, and the test
// transport override. Returns the proxy-parse error from getClientForOptions
// unchanged; callers wrap it.
func buildFetchClient(opts Options, domain string, tracker *redirectTracker) (*http.Client, error) {
	timeout := 30 * time.Second
	if opts.DomainTimeout != nil && domain != "" {
		if domainTimeout, ok := opts.DomainTimeout[domain]; ok && domainTimeout > 0 {
			timeout = domainTimeout
		}
	}

	// CheckRedirect: a SecurityPolicy enforces its own MaxRedirects + per-hop
	// revalidation; otherwise the default 10-hop tracker applies.
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
		// Honour proxy even in the no-pooling / per-domain-timeout branch.
		if opts.Proxy != "" {
			client.Transport = sharedC.Transport
		}
	} else {
		client = &http.Client{
			Timeout:       sharedC.Timeout,
			Transport:     sharedC.Transport,
			CheckRedirect: checkRedirect,
		}
	}
	// Security dial guard: on the direct (non-proxy) path, swap the cached
	// singleton transport for a fresh one carrying the policy so its dial
	// Control hook can re-validate the resolved IP. The proxy path keeps its
	// proxy-aware transport (target IP is vetted by ValidateURL instead).
	if opts.Security != nil && opts.Proxy == "" {
		client.Transport = &chromeTransport{security: opts.Security}
	}
	// Test injection: an opts-supplied RoundTripper trumps the utls/proxy
	// transport. Lets mock.Replay/mock.Record serve canned bytes without
	// touching the network.
	if opts.Transport != nil {
		client.Transport = opts.Transport
	}

	return client, nil
}

// resolveUserAgentFor resolves the effective User-Agent: opts.UserAgent
// (through the named-persona table), overridden by any per-domain entry.
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

// robotsFetchOptions builds the robots.txt fetch template from extraction
// Options so a robots fetch rides the same client path as the page fetch:
// proxy honoured, security policy applied, and the Transport test seam
// respected (previously a raw private http.Client bypassed all three).
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

// checkRobotsAllowed applies the RespectRobots gate. When the target is
// disallowed it marks result blocked-by-robots and returns false; result is
// untouched (and fetching may proceed) otherwise.
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
	result.Error = "blocked by robots.txt"
	result.BlockedByRobots = true
	ensureProfile(result)
	result.Profile.Reasons = append(result.Profile.Reasons, "blocked-by-robots")
	return false
}

// applyCrawlDelay honours a robots.txt Crawl-delay for the target host,
// waiting ctx-aware so a deadline or SIGINT can preempt the sleep (ALP-043).
// The effective delay is clamped to maxCrawlDelay; a non-empty warning is
// returned when the clamp fired so the caller can surface it on the result.
func applyCrawlDelay(ctx context.Context, opts Options, targetURL, domain, userAgent string) (warning string, err error) {
	if !opts.RespectCrawlDelay || domain == "" {
		return "", nil
	}
	crawlCache := opts.CrawlDelayCache
	if crawlCache == nil {
		crawlCache = newCrawlDelayCacheWithFetch(robotsFetchOptions(opts))
	}
	// Use parsed.Host (host[:port]) instead of domain (hostname only) so the
	// cache fetches robots.txt from the right port and keys per-port. Mirrors
	// the RespectRobots gate. Without this, non-default-port targets
	// (httptest, local dev, intranet :8080) silently get no delay enforcement.
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

// applyRateLimit enforces opts.RateLimit spacing for the domain, ctx-aware.
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
