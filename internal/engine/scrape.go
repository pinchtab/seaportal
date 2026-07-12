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

// ErrNotImplemented is returned by ScrapeSite until the real discovery,
// sampling, and extraction pipeline (ALP-002…008) lands. The types and the
// ScrapeSite signature are stable now so callers (CLI, PinchTab) can compile
// against them.
var ErrNotImplemented = errors.New("seaportal: ScrapeSite not implemented")

// ErrMissingBaseURL is returned when ScrapeOptions.BaseURL is empty.
var ErrMissingBaseURL = errors.New("seaportal: ScrapeOptions.BaseURL is required")

// ErrInvalidBaseURL is returned when ScrapeOptions.BaseURL is set but is not a
// valid absolute URL (e.g. a bare host like "example.com" missing its scheme),
// as opposed to ErrMissingBaseURL for a genuinely empty value.
var ErrInvalidBaseURL = errors.New("seaportal: ScrapeOptions.BaseURL is not a valid absolute URL")

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

// discoveryTimeoutFraction is the share of the overall scrape --timeout the
// discovery stage may use before its context is cancelled, reserving the rest
// for fetching so a link-dense crawl-fallback can't starve the fetch phase
// (ALP-040).
const discoveryTimeoutFraction = 0.5

// discoveryBudget returns the discovery stage's slice of the overall timeout.
func discoveryBudget(total time.Duration) time.Duration {
	return time.Duration(float64(total) * discoveryTimeoutFraction)
}

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

	// Security is the fetch policy applied to every scrape network call:
	// discovery (robots.txt, sitemaps, the crawl fallback) and the per-page
	// fetches. Unlike single-URL extraction — where a nil policy preserves the
	// historical unguarded behaviour — ScrapeSite is secure by default:
	// normalized() replaces nil with DefaultSecurityPolicy(). Callers that
	// must crawl private/internal hosts pass an explicit policy with
	// BlockPrivateIPs disabled (the CLI --allow-internal / MCP allow_internal
	// escape hatch).
	Security *SecurityPolicy
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
	if n.Security == nil {
		// Secure by default: ScrapeSite is a crawling entry point fed with
		// arbitrary base URLs (MCP scrape_site, CLI), so a nil policy gets the
		// full default guard rather than the extract path's historical
		// nil-means-unguarded semantics.
		n.Security = DefaultSecurityPolicy()
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
	// UnsampledPatterns counts discovered URL patterns that contributed no
	// sampled page (filtered out or over budget) and are therefore omitted
	// from pageGroups.
	UnsampledPatterns int `json:"unsampledPatterns,omitempty"`
	// Warnings carries non-fatal politeness/safety notices from the run,
	// e.g. a hostile robots.txt Crawl-delay clamped to maxCrawlDelay.
	Warnings []string `json:"warnings,omitempty"`
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
		return nil, fmt.Errorf("%w: %q", ErrInvalidBaseURL, o.BaseURL)
	}

	// One overall wall-clock deadline shared by discovery, fetch, and retries.
	// The raw (pre-normalization) Timeout is used so an explicit 0 keeps the
	// no-overall-deadline escape; normalized o.Timeout still caps each request.
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// One robots cache + one host rate limiter for the whole run (T03):
	// previously discovery, the crawl fallback, and the fetch phase each built
	// their own cache, re-fetching robots.txt up to 3× per run.
	robots := newCrawlDelayCacheWithFetch(FetchBytesOptions{Security: o.Security})
	limiter := NewHostRateLimiter()

	// Sub-budget discovery so a link-dense crawl-fallback can't consume the
	// whole deadline and starve fetching (ALP-040). discover() returns whatever
	// it found when its context is cancelled, so the reserved remainder is left
	// for fetchAndAssemble on the still-live overall ctx (which stays the
	// wall-clock cap — ALP-029).
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

	// Only patterns that contributed a sampled page appear in pageGroups;
	// filtered-out and budget-skipped patterns are rolled up into
	// summary.unsampledPatterns instead of shipping as empty shells.
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

	return &ScrapeResult{
		Site:       site,
		PageGroups: outGroups,
		Pages:      pages,
		Summary:    summary,
	}, nil
}

// fetchAndAssemble fetches and extracts each URL once through the full
// FromURLWithOptions pipeline (retries, disk cache, redirect tracking, charset
// recovery, security policy — T07; the raw FetchBytes+FromHTMLWithOptions
// shortcut bypassed all of those) and assembles a PageObject per URL,
// concurrently with per-host rate limiting and robots crawl-delay via the
// run-shared robots cache and limiter (T03). A hostile Crawl-delay is clamped
// to maxCrawlDelay and reported once per host in the returned warnings (T05).
// Partial failures are captured on the page's Error field.
func fetchAndAssemble(ctx context.Context, base *url.URL, urls []string, o ScrapeOptions, robots *CrawlDelayCache, limiter *HostRateLimiter) ([]PageObject, []string) {
	respectRobots := o.RespectRobots != nil && *o.RespectRobots
	withPerf := o.WithPerformance

	// WithLinks feeds assemblePage's internal/external link counts; the shared
	// CrawlDelayCache keeps the in-pipeline robots gate one-fetch-per-host.
	// RespectCrawlDelay stays off — spacing is enforced by limiter.Wait below,
	// which reserves per-host slots instead of sleeping per request.
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
		opts := template
		// Bound the fetch (including retry backoff waits) by the pool ctx so a
		// page dispatched just before the deadline can't run past it.
		opts.Context = ctx
		return assemblePage(base, u, FromURLWithOptions(u, opts), withPerf)
	})
	return pages, warnings
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
