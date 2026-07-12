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
	// ErrMissingBaseURL is returned when ScrapeOptions.BaseURL is empty.
	ErrMissingBaseURL = engine.ErrMissingBaseURL
)

// ScrapeSite runs the full scrape pipeline starting from opts.BaseURL:
// discover candidate URLs (robots/sitemap/crawl fallback), cluster them into
// pattern groups, sample within budget, fetch + extract each page, and roll
// up a site summary. When the caller's ctx is cancelled mid-run, the partial
// result is returned alongside ctx.Err() (both non-nil) rather than dropped;
// the internal opts.Timeout budget elapsing is a normal (nil-error)
// completion.
func ScrapeSite(ctx context.Context, opts *ScrapeOptions) (*ScrapeResult, error) {
	return engine.ScrapeSite(ctx, opts)
}

// RenderScrapeJSON marshals a ScrapeResult to indented JSON (the default
// `--output json`).
func RenderScrapeJSON(res *ScrapeResult) ([]byte, error) {
	return engine.RenderScrapeJSON(res)
}

// RenderScrapeMarkdown renders a ScrapeResult as a single Markdown digest
// (`--output md`).
func RenderScrapeMarkdown(res *ScrapeResult) string {
	return engine.RenderScrapeMarkdown(res)
}

// WriteScrapeDirectory writes a ScrapeResult as a directory of assets
// (`--output directory`): result.json, pages/<slug>.md, and an index.md
// manifest. Returns the relative page file paths.
func WriteScrapeDirectory(res *ScrapeResult, dir string) ([]string, error) {
	return engine.WriteScrapeDirectory(res, dir)
}
