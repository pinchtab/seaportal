package engine

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

// ErrNotImplemented is returned by ScrapeSite until the real discovery,
// sampling, and extraction pipeline (ALP-002…008) lands. The types and the
// ScrapeSite signature are stable now so callers (CLI, PinchTab) can compile
// against them.
var ErrNotImplemented = errors.New("seaportal: ScrapeSite not implemented")

// ErrMissingBaseURL is returned when ScrapeOptions.BaseURL is empty.
var ErrMissingBaseURL = errors.New("seaportal: ScrapeOptions.BaseURL is required")

// SampleStrategy selects how pages are sampled within a URL pattern group.
type SampleStrategy string

const (
	// SampleBalanced spreads samples across groups (default).
	SampleBalanced SampleStrategy = "balanced"
	// SampleRandom picks samples at random within a group.
	SampleRandom SampleStrategy = "random"
	// SamplePriority favours the homepage and main sections first.
	SamplePriority SampleStrategy = "priority"
)

// OutputFormat selects how ScrapeSite results are rendered by the CLI.
type OutputFormat string

const (
	// OutputJSON emits the ScrapeResult as JSON (default).
	OutputJSON OutputFormat = "json"
	// OutputMarkdown emits a Markdown report.
	OutputMarkdown OutputFormat = "md"
	// OutputDirectory writes a directory with assets.
	OutputDirectory OutputFormat = "directory"
)

// Documented ScrapeOptions defaults (see the scrape spec).
const (
	DefaultScrapeMaxPages      = 50
	DefaultScrapeMaxPerPattern = 8
	DefaultScrapeTimeout       = 60 * time.Second
)

// ScrapeOptions controls ScrapeSite. Every field maps to a `seaportal scrape`
// flag; zero-valued fields resolve to the documented defaults in normalized().
//
// RespectRobots is a *bool so an unset option (nil) can default to true while
// still letting a caller explicitly opt out with a pointer to false.
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
}

// normalized returns a copy of o with zero-valued fields replaced by the
// documented defaults. It never mutates the receiver.
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
	return n
}

// validate reports whether the caller-supplied options are usable.
func (o ScrapeOptions) validate() error {
	if strings.TrimSpace(o.BaseURL) == "" {
		return ErrMissingBaseURL
	}
	return nil
}

// SiteInfo is the top-level `site` object in a ScrapeResult.
type SiteInfo struct {
	BaseURL            string    `json:"baseURL"`
	Title              string    `json:"title"`
	DiscoveredAt       time.Time `json:"discoveredAt"`
	SitemapFound       bool      `json:"sitemapFound"`
	TotalURLsInSitemap int       `json:"totalURLsInSitemap"`
	SampledPages       int       `json:"sampledPages"`
}

// PageGroup is one clustered URL pattern (e.g. `/blog/*`) with its samples.
type PageGroup struct {
	Pattern        string       `json:"pattern"`
	TotalInSitemap int          `json:"totalInSitemap"`
	Sampled        int          `json:"sampled"`
	Pages          []PageObject `json:"pages"`
}

// PagePerformance holds the basic per-page performance metrics.
type PagePerformance struct {
	TTFBMillis int64 `json:"ttfbMs"`
	TotalBytes int64 `json:"totalBytes"`
	Requests   int   `json:"requests"`
}

// PageObject is one extracted page in a ScrapeResult.
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
	// Error is set when this page failed to fetch or extract; a partial
	// failure is recorded here rather than aborting the whole scrape.
	Error string `json:"error,omitempty"`
}

// ScrapeSummary is the roll-up `summary` object in a ScrapeResult.
type ScrapeSummary struct {
	ContentTypes    map[string]int `json:"contentTypes"`
	Recommendations []string       `json:"recommendations"`
}

// ScrapeResult is the structured output of ScrapeSite, matching the scrape
// spec's JSON shape.
type ScrapeResult struct {
	Site       SiteInfo      `json:"site"`
	PageGroups []PageGroup   `json:"pageGroups"`
	Pages      []PageObject  `json:"pages"`
	Summary    ScrapeSummary `json:"summary"`
}

// ScrapeSite runs the full scrape pipeline for opts.BaseURL: discover candidate
// URLs, cluster them into pattern groups, sample within budget, fetch + extract
// + assemble each page concurrently, and roll up a site summary.
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
		return nil, ErrMissingBaseURL
	}

	// One overall wall-clock deadline shared by discovery, fetch, and retries.
	// The raw (pre-normalization) Timeout is used so an explicit 0 keeps the
	// no-overall-deadline escape; normalized o.Timeout still caps each request.
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	disc, err := discover(ctx, o)
	if err != nil {
		return nil, err
	}
	groups := groupByPattern(disc.URLs)
	sampled := sample(groups, o)
	pages := fetchAndAssemble(ctx, base, sampled, o)

	sampledSet := make(map[string]bool, len(sampled))
	for _, u := range sampled {
		sampledSet[u] = true
	}
	pageByURL := make(map[string]PageObject, len(pages))
	for _, p := range pages {
		pageByURL[p.URL] = p
	}

	outGroups := make([]PageGroup, 0, len(groups))
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

	return &ScrapeResult{
		Site:       site,
		PageGroups: outGroups,
		Pages:      pages,
		Summary:    summarize(pages, disc.TotalURLsInSitemap, len(groups)),
	}, nil
}

// fetchAndAssemble fetches each URL's HTML once and assembles a full PageObject
// (ALP-006), concurrently with per-host rate limiting and robots crawl-delay.
// Partial failures are captured on the page's Error field.
func fetchAndAssemble(ctx context.Context, base *url.URL, urls []string, o ScrapeOptions) []PageObject {
	respectRobots := o.RespectRobots != nil && *o.RespectRobots
	withPerf := o.WithPerformance
	limiter := NewHostRateLimiter()
	robots := NewCrawlDelayCache()

	return runFetchPool(ctx, urls, o.Timeout, defaultScrapeConcurrency, func(ctx context.Context, u string) PageObject {
		if err := ctx.Err(); err != nil {
			return PageObject{URL: u, Error: err.Error()}
		}
		host, scheme := hostScheme(u)
		if respectRobots && host != "" {
			limiter.Wait(host, robots.GetDelayWithScheme(host, o.UserAgent, scheme))
		}
		// TTFB here is the whole FetchBytes round-trip (headers + body):
		// FetchBytes exposes no first-byte hook, and > 0 beats the structural 0
		// this path used to report.
		fetchStart := time.Now()
		body, _, status, err := FetchBytes(ctx, u, FetchBytesOptions{Timeout: o.Timeout, UserAgent: o.UserAgent})
		fetchMs := time.Since(fetchStart).Milliseconds()
		if err != nil {
			return PageObject{URL: u, Status: status, Error: err.Error()}
		}
		r := FromHTMLWithOptions(string(body), u, Options{UserAgent: o.UserAgent})
		r.StatusCode = status
		r.TTFBMs = fetchMs
		return assemblePage(base, u, string(body), r, withPerf)
	})
}

// siteTitle picks the homepage title when present, else the first non-empty
// page title.
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
