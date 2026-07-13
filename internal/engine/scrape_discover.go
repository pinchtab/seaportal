package engine

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

const defaultCrawlDepth = 2

type discoveryResult struct {
	URLs               []string
	SitemapFound       bool
	TotalURLsInSitemap int
}

var reSitemapDirective = regexp.MustCompile(`(?im)^\s*sitemap:\s*(\S+)`)

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
			return
		}
		if !allowed(cu) || seen[cu] {
			return
		}
		seen[cu] = true
		candidates = append(candidates, cu)
	}

	for _, sm := range discoverSitemapURLs(ctx, scheme, host, o) {
		if ctx.Err() != nil {
			break
		}
		entries, ferr := FlattenSitemap(ctx, sm, FlattenSitemapOptions{Timeout: o.Timeout, Security: o.Security, Since: o.Since})
		if len(entries) > 0 {
			res.SitemapFound = true
			res.TotalURLsInSitemap += len(entries)
			for _, e := range entries {
				add(e.Loc)
			}
		}
		if ferr != nil {
			if ctx.Err() != nil || errors.Is(ferr, context.DeadlineExceeded) || errors.Is(ferr, context.Canceled) {
				break
			}
			continue
		}
	}

	if !res.SitemapFound {
		for _, u := range crawlSameHost(ctx, scheme+"://"+host+"/", host, o, robots, limiter, o.MaxPages, defaultCrawlDepth) {
			add(u)
		}
	}

	res.URLs = candidates
	return res, nil
}

func discoverSitemapURLs(ctx context.Context, scheme, host string, o ScrapeOptions) []string {
	var out []string
	seen := map[string]bool{}
	push := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" {
			return
		}
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
				break
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
				continue
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
