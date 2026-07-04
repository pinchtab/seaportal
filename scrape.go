package seaportal

import (
	"context"

	"github.com/pinchtab/seaportal/internal/engine"
)

// ScrapeOptions controls ScrapeSite behaviour (discovery, sampling, extraction).
type ScrapeOptions = engine.ScrapeOptions

// ScrapeResult is the structured output of ScrapeSite.
type ScrapeResult = engine.ScrapeResult

// SiteInfo is the top-level `site` object in a ScrapeResult.
type SiteInfo = engine.SiteInfo

// PageGroup is one clustered URL pattern with its sampled pages.
type PageGroup = engine.PageGroup

// PageObject is one extracted page in a ScrapeResult.
type PageObject = engine.PageObject

// PagePerformance holds the basic per-page performance metrics.
type PagePerformance = engine.PagePerformance

// ScrapeSummary is the roll-up `summary` object in a ScrapeResult.
type ScrapeSummary = engine.ScrapeSummary

// SampleStrategy selects how pages are sampled within a URL pattern group.
type SampleStrategy = engine.SampleStrategy

// OutputFormat selects how ScrapeSite results are rendered.
type OutputFormat = engine.OutputFormat

// Sampling strategies for ScrapeOptions.SampleStrategy.
const (
	SampleBalanced = engine.SampleBalanced
	SampleRandom   = engine.SampleRandom
	SamplePriority = engine.SamplePriority
)

// Output formats for ScrapeOptions.Output.
const (
	OutputJSON      = engine.OutputJSON
	OutputMarkdown  = engine.OutputMarkdown
	OutputDirectory = engine.OutputDirectory
)

// Documented ScrapeOptions defaults.
const (
	DefaultScrapeMaxPages      = engine.DefaultScrapeMaxPages
	DefaultScrapeMaxPerPattern = engine.DefaultScrapeMaxPerPattern
	DefaultScrapeTimeout       = engine.DefaultScrapeTimeout
)

// Sentinel errors returned by ScrapeSite.
var (
	// ErrNotImplemented is returned by ScrapeSite until the pipeline lands.
	ErrNotImplemented = engine.ErrNotImplemented
	// ErrMissingBaseURL is returned when ScrapeOptions.BaseURL is empty.
	ErrMissingBaseURL = engine.ErrMissingBaseURL
)

// ScrapeSite discovers, samples, and extracts a whole site starting from
// opts.BaseURL. It validates options and applies defaults; the extraction
// pipeline is not yet implemented (returns ErrNotImplemented).
func ScrapeSite(ctx context.Context, opts *ScrapeOptions) (*ScrapeResult, error) {
	return engine.ScrapeSite(ctx, opts)
}
