package engine

import (
	"context"
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
		pu, perr := url.Parse(cu)
		if perr != nil || !strings.EqualFold(pu.Host, host) {
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
		entries, ferr := FlattenSitemap(ctx, sm, FlattenSitemapOptions{Timeout: o.Timeout, Security: o.Security})
		if ferr != nil || len(entries) == 0 {
			continue
		}
		res.SitemapFound = true
		res.TotalURLsInSitemap += len(entries)
		for _, e := range entries {
			add(e.Loc)
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
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
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
			pu, perr := url.Parse(nu)
			if perr != nil || !strings.EqualFold(pu.Host, host) {
				continue // same-host only; external excluded
			}
			if respectRobots && !robots.IsAllowed(ctx, pu.Host, o.UserAgent, pu.Scheme, pu.Path) {
				continue
			}
			visited[nu] = true
			queue = append(queue, item{nu, cur.depth + 1})
		}
	}
	return found
}
