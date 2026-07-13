package engine

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// defaultCrawlDepth bounds the no-sitemap homepage crawl fallback.
const defaultCrawlDepth = 2

// discoveryResult is the output of the ScrapeSite discovery stage: the
// de-duplicated, same-host candidate URL set the sampler (ALP-004) draws from,
// plus sitemap metadata for SiteInfo.
type discoveryResult struct {
	URLs               []string
	SitemapFound       bool
	TotalURLsInSitemap int
}

var reSitemapDirective = regexp.MustCompile(`(?im)^\s*sitemap:\s*(\S+)`)

// discover runs the discovery stage for opts.BaseURL: read Sitemap directives
// from robots.txt plus the conventional /sitemap.xml, flatten sitemap indexes
// via FlattenSitemap, and — if no sitemap yields URLs — fall back to a bounded
// same-host crawl seeded from the homepage. When RespectRobots is set,
// disallowed paths are dropped from the candidate set. robots and limiter are
// the run-shared cache/limiter created by ScrapeSite (T03) so robots.txt is
// fetched once per host across discovery and the fetch phase.
func discover(ctx context.Context, opts ScrapeOptions, robots *CrawlDelayCache, limiter *HostRateLimiter) (discoveryResult, error) {
	o := opts.normalized()
	base, err := url.Parse(strings.TrimSpace(o.BaseURL))
	if err != nil || base.Host == "" {
		return discoveryResult{}, ErrMissingBaseURL
	}
	host := strings.ToLower(base.Host)
	scheme := base.Scheme
	if scheme == "" {
		scheme = "https"
	}

	respectRobots := o.RespectRobots != nil && *o.RespectRobots
	allowed := func(rawURL string) bool {
		if !respectRobots {
			return true
		}
		u, perr := url.Parse(rawURL)
		if perr != nil {
			return false
		}
		return robots.IsAllowed(ctx, u.Host, o.UserAgent, u.Scheme, u.Path)
	}

	res := discoveryResult{}
	seen := map[string]bool{}
	var candidates []string
	add := func(raw string) {
		cu, _, ok := normalizeMember(raw)
		if !ok || cu == "" {
			return
		}
		if !sameHost(cu, host, scheme) {
			return // drop external hosts
		}
		if !allowed(cu) || seen[cu] {
			return
		}
		seen[cu] = true
		candidates = append(candidates, cu)
	}

	// 1+2. Sitemaps discovered from robots.txt and the conventional location,
	// flattened (indexes included) via the existing FlattenSitemap.
	for _, sm := range discoverSitemapURLs(ctx, scheme, host, o) {
		if ctx.Err() != nil {
			break
		}
		entries, ferr := FlattenSitemap(ctx, sm, FlattenSitemapOptions{Timeout: o.Timeout, Security: o.Security, Since: o.Since})
		// Keep whatever was flattened even when the deadline fired mid-walk:
		// a partial sitemap is still useful discovery output (ALP-051).
		if len(entries) > 0 {
			res.SitemapFound = true
			res.TotalURLsInSitemap += len(entries)
			for _, e := range entries {
				add(e.Loc)
			}
		}
		if ferr != nil {
			// A deadline/cancellation means the discovery budget is spent —
			// stop rather than grinding through the remaining sitemaps (each
			// would just fail its first fetch). A per-sitemap fetch/parse error
			// is local: skip only that sitemap.
			if ctx.Err() != nil || errors.Is(ferr, context.DeadlineExceeded) || errors.Is(ferr, context.Canceled) {
				break
			}
			continue
		}
	}

	// 3. Crawl fallback when no sitemap produced any URLs.
	if !res.SitemapFound {
		for _, u := range crawlSameHost(ctx, scheme+"://"+host+"/", host, o, robots, limiter, o.MaxPages, defaultCrawlDepth) {
			add(u)
		}
	}

	res.URLs = candidates
	return res, nil
}

// discoverSitemapURLs returns the sitemap URLs to try: every `Sitemap:`
// directive in robots.txt, followed by the conventional /sitemap.xml.
func discoverSitemapURLs(ctx context.Context, scheme, host string, o ScrapeOptions) []string {
	var out []string
	seen := map[string]bool{}
	push := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" {
			return
		}
		// Dedup on a port-canonical key so a robots `Sitemap:` directive that
		// omits the default port and the conventional `/sitemap.xml` built from
		// a base host that includes it are recognised as the same sitemap and
		// not fetched (and flattened) twice.
		key := u
		if pu, err := url.Parse(u); err == nil {
			key = canonicalHost(pu.Host, pu.Scheme) + pu.Path
		}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, u)
	}

	body, _, status, err := FetchBytes(ctx, scheme+"://"+host+"/robots.txt", FetchBytesOptions{
		Timeout:   o.Timeout,
		UserAgent: o.UserAgent,
		Security:  o.Security,
	})
	if err == nil && status == 200 {
		for _, m := range reSitemapDirective.FindAllStringSubmatch(string(body), -1) {
			push(m[1])
		}
	}
	push(scheme + "://" + host + "/sitemap.xml")
	return out
}

// crawlSameHost does a bounded breadth-first crawl from seed, following only
// same-host links, up to maxURLs pages and maxDepth deep. Disallowed paths are
// skipped when RespectRobots is set, and each crawl fetch honours the robots
// crawl-delay (clamped) through the run-shared limiter — the BFS previously
// hammered the host with no spacing at all (T03). Returns the visited URLs in
// BFS order.
func crawlSameHost(ctx context.Context, seed, host string, o ScrapeOptions, robots *CrawlDelayCache, limiter *HostRateLimiter, maxURLs, maxDepth int) []string {
	if maxURLs <= 0 {
		maxURLs = DefaultScrapeMaxPages
	}
	if maxDepth < 0 {
		maxDepth = 0
	}
	respectRobots := o.RespectRobots != nil && *o.RespectRobots
	_, baseScheme := hostScheme(seed)

	type item struct {
		url   string
		depth int
	}
	start, _, ok := normalizeMember(seed)
	if !ok {
		return nil
	}
	queue := []item{{start, 0}}
	visited := map[string]bool{start: true}
	var found []string

	for len(queue) > 0 && len(found) < maxURLs {
		if ctx.Err() != nil {
			break
		}
		cur := queue[0]
		queue = queue[1:]
		found = append(found, cur.url)
		if cur.depth >= maxDepth {
			continue
		}
		if respectRobots {
			curHost, curScheme := hostScheme(cur.url)
			delay, _ := clampCrawlDelay(robots.GetDelayWithScheme(ctx, curHost, o.UserAgent, curScheme))
			if limiter.Wait(ctx, curHost, delay) != nil {
				break // ctx fired while waiting for the slot
			}
		}
		body, _, status, err := FetchBytes(ctx, cur.url, FetchBytesOptions{
			Timeout:   o.Timeout,
			UserAgent: o.UserAgent,
			Security:  o.Security,
		})
		if err != nil || status != 200 {
			continue
		}
		for _, l := range ExtractLinks(string(body), cur.url) {
			nu, _, ok := normalizeMember(l.Href)
			if !ok || visited[nu] {
				continue
			}
			if !sameHost(nu, host, baseScheme) {
				continue // same-host only; external excluded
			}
			if respectRobots {
				pu, perr := url.Parse(nu)
				if perr != nil || !robots.IsAllowed(ctx, pu.Host, o.UserAgent, pu.Scheme, pu.Path) {
					continue
				}
			}
			visited[nu] = true
			queue = append(queue, item{nu, cur.depth + 1})
		}
	}
	return found
}
