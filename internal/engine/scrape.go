package engine

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrMissingBaseURL = errors.New("seaportal: ScrapeOptions.BaseURL is required")

var ErrInvalidBaseURL = errors.New("seaportal: ScrapeOptions.BaseURL is not a valid absolute URL")

type SampleStrategy string

const (
	SampleBalanced SampleStrategy = "balanced"
	SampleRandom   SampleStrategy = "random"
	SamplePriority SampleStrategy = "priority"
)

type OutputFormat string

const (
	OutputJSON      OutputFormat = "json"
	OutputMarkdown  OutputFormat = "md"
	OutputDirectory OutputFormat = "directory"
)

const (
	DefaultScrapeMaxPages      = 50
	DefaultScrapeMaxPerPattern = 8
	DefaultScrapeTimeout       = 60 * time.Second
)

const discoveryTimeoutFraction = 0.5

func discoveryBudget(total time.Duration) time.Duration {
	return time.Duration(float64(total) * discoveryTimeoutFraction)
}

type ScrapeOptions struct {
	BaseURL         string
	MaxPages        int
	MaxPerPattern   int
	Full            bool
	IncludePatterns []string
	ExcludePatterns []string
	SampleStrategy  SampleStrategy
	Output          OutputFormat
	WithPerformance bool
	RespectRobots   *bool
	Timeout         time.Duration
	UserAgent       string

	Security *SecurityPolicy

	Since time.Time
}

func (o ScrapeOptions) normalized() ScrapeOptions {
	n := o
	if n.MaxPages <= 0 {
		n.MaxPages = DefaultScrapeMaxPages
	}
	if n.MaxPerPattern <= 0 {
		n.MaxPerPattern = DefaultScrapeMaxPerPattern
	}
	if n.SampleStrategy == "" {
		n.SampleStrategy = SampleBalanced
	}
	if n.Output == "" {
		n.Output = OutputJSON
	}
	if n.Timeout <= 0 {
		n.Timeout = DefaultScrapeTimeout
	}
	if n.RespectRobots == nil {
		t := true
		n.RespectRobots = &t
	}
	if n.UserAgent == "" {
		n.UserAgent = DefaultUserAgent
	}
	if n.Security == nil {
		n.Security = DefaultSecurityPolicy()
	}
	return n
}

func (o ScrapeOptions) validate() error {
	if strings.TrimSpace(o.BaseURL) == "" {
		return ErrMissingBaseURL
	}
	return nil
}

type SiteInfo struct {
	BaseURL            string    `json:"baseURL"`
	Title              string    `json:"title"`
	DiscoveredAt       time.Time `json:"discoveredAt"`
	SitemapFound       bool      `json:"sitemapFound"`
	TotalURLsInSitemap int       `json:"totalURLsInSitemap"`
	SampledPages       int       `json:"sampledPages"`
}

type PageGroup struct {
	Pattern        string       `json:"pattern"`
	TotalInSitemap int          `json:"totalInSitemap"`
	Sampled        int          `json:"sampled"`
	Pages          []PageObject `json:"pages"`
}

type PagePerformance struct {
	TTFBMillis int64 `json:"ttfbMs"`
	TotalBytes int64 `json:"totalBytes"`
	Requests   int   `json:"requests"`
}

type PageObject struct {
	URL           string            `json:"url"`
	Title         string            `json:"title"`
	Status        int               `json:"status"`
	Meta          map[string]string `json:"meta,omitempty"`
	Markdown      string            `json:"markdown"`
	Schema        []map[string]any  `json:"schema,omitempty"`
	Performance   *PagePerformance  `json:"performance,omitempty"`
	ContentType   string            `json:"contentType"`
	InternalLinks int               `json:"internalLinks"`
	ExternalLinks int               `json:"externalLinks"`
	Error         string            `json:"error,omitempty"`
}

type ScrapeSummary struct {
	ContentTypes      map[string]int `json:"contentTypes"`
	Recommendations   []string       `json:"recommendations"`
	UnsampledPatterns int            `json:"unsampledPatterns,omitempty"`
	Warnings          []string       `json:"warnings,omitempty"`
}

type ScrapeResult struct {
	Site       SiteInfo      `json:"site"`
	PageGroups []PageGroup   `json:"pageGroups"`
	Pages      []PageObject  `json:"pages"`
	Summary    ScrapeSummary `json:"summary"`
}

func ScrapeSite(ctx context.Context, opts *ScrapeOptions) (*ScrapeResult, error) {
	if opts == nil {
		return nil, ErrMissingBaseURL
	}
	if err := opts.validate(); err != nil {
		return nil, err
	}
	o := opts.normalized()
	base, err := url.Parse(o.BaseURL)
	if err != nil || base.Host == "" {
		return nil, fmt.Errorf("%w: %q", ErrInvalidBaseURL, o.BaseURL)
	}

	callerCtx := ctx

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	robots := newCrawlDelayCacheWithFetch(FetchBytesOptions{Security: o.Security})
	limiter := NewHostRateLimiter()

	discCtx := ctx
	if opts.Timeout > 0 {
		var discCancel context.CancelFunc
		discCtx, discCancel = context.WithTimeout(ctx, discoveryBudget(opts.Timeout))
		defer discCancel()
	}

	disc, err := discover(discCtx, o, robots, limiter)
	if err != nil {
		return nil, err
	}
	groups := groupByPattern(disc.URLs)
	sampled := sample(groups, o)
	pages, fetchWarnings := fetchAndAssemble(ctx, base, sampled, o, robots, limiter)

	sampledSet := make(map[string]bool, len(sampled))
	for _, u := range sampled {
		sampledSet[u] = true
	}
	pageByURL := make(map[string]PageObject, len(pages))
	for _, p := range pages {
		pageByURL[p.URL] = p
	}

	outGroups := make([]PageGroup, 0, len(groups))
	unsampled := 0
	for _, g := range groups {
		grp := PageGroup{Pattern: g.Pattern, TotalInSitemap: g.TotalInSitemap, Pages: []PageObject{}}
		for _, u := range g.URLs {
			if sampledSet[u] {
				grp.Sampled++
				if p, ok := pageByURL[u]; ok {
					grp.Pages = append(grp.Pages, p)
				}
			}
		}
		if grp.Sampled == 0 {
			unsampled++
			continue
		}
		outGroups = append(outGroups, grp)
	}

	site := SiteInfo{
		BaseURL:            o.BaseURL,
		Title:              siteTitle(pages),
		DiscoveredAt:       time.Now().UTC(),
		SitemapFound:       disc.SitemapFound,
		TotalURLsInSitemap: disc.TotalURLsInSitemap,
		SampledPages:       len(pages),
	}

	summary := summarize(pages, disc.TotalURLsInSitemap, len(groups))
	summary.UnsampledPatterns = unsampled
	summary.Warnings = fetchWarnings
	if len(pages) == 0 && len(disc.URLs) > 0 {
		summary.Recommendations = append(summary.Recommendations,
			fmt.Sprintf("0 of %d discovered URLs were sampled; check --include-patterns/--exclude-patterns and --max-pages", len(disc.URLs)))
	}

	res := &ScrapeResult{
		Site:       site,
		PageGroups: outGroups,
		Pages:      pages,
		Summary:    summary,
	}
	if cerr := callerCtx.Err(); cerr != nil {
		return res, cerr
	}
	return res, nil
}

func fetchAndAssemble(ctx context.Context, base *url.URL, urls []string, o ScrapeOptions, robots *CrawlDelayCache, limiter *HostRateLimiter) ([]PageObject, []string) {
	respectRobots := o.RespectRobots != nil && *o.RespectRobots
	withPerf := o.WithPerformance

	template := Options{
		UserAgent:       o.UserAgent,
		Security:        o.Security,
		WithLinks:       true,
		RespectRobots:   respectRobots,
		CrawlDelayCache: robots,
	}

	var warnMu sync.Mutex
	var warnings []string
	clampWarned := map[string]bool{}

	pages := runFetchPool(ctx, urls, o.Timeout, defaultScrapeConcurrency, func(ctx context.Context, u string) PageObject {
		if err := ctx.Err(); err != nil {
			return PageObject{URL: u, Error: err.Error()}
		}
		host, scheme := hostScheme(u)
		if respectRobots && host != "" {
			delay := robots.GetDelayWithScheme(ctx, host, o.UserAgent, scheme)
			if capped, clamped := clampCrawlDelay(delay); clamped {
				warnMu.Lock()
				if !clampWarned[host] {
					clampWarned[host] = true
					warnings = append(warnings, fmt.Sprintf("robots.txt crawl-delay %s for %s clamped to %s", delay, host, maxCrawlDelay))
				}
				warnMu.Unlock()
				delay = capped
			}
			if err := limiter.Wait(ctx, host, delay); err != nil {
				return PageObject{URL: u, Error: err.Error()}
			}
		}
		return assemblePage(base, u, FromURLContext(ctx, u, template), withPerf)
	})
	return pages, warnings
}

func siteTitle(pages []PageObject) string {
	for _, p := range pages {
		if pu, err := url.Parse(p.URL); err == nil && (pu.Path == "" || pu.Path == "/") && p.Title != "" {
			return p.Title
		}
	}
	for _, p := range pages {
		if p.Title != "" {
			return p.Title
		}
	}
	return ""
}
