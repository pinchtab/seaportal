package engine

import (
	"context"
	"errors"
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

// ScrapeSite discovers, samples, and extracts a whole site. It validates opts,
// applies defaults, and currently returns ErrNotImplemented — the real pipeline
// lands in ALP-002…008.
func ScrapeSite(ctx context.Context, opts *ScrapeOptions) (*ScrapeResult, error) {
	if opts == nil {
		return nil, ErrMissingBaseURL
	}
	if err := opts.validate(); err != nil {
		return nil, err
	}
	_ = opts.normalized()
	_ = ctx
	return nil, ErrNotImplemented
}
