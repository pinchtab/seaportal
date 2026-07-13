package seaportal

import (
	"context"

	"github.com/pinchtab/seaportal/internal/engine"
)

type ScrapeOptions = engine.ScrapeOptions

type ScrapeResult = engine.ScrapeResult

type SiteInfo = engine.SiteInfo

type PageGroup = engine.PageGroup

type PageObject = engine.PageObject

type PagePerformance = engine.PagePerformance

type ScrapeSummary = engine.ScrapeSummary

type SampleStrategy = engine.SampleStrategy

type OutputFormat = engine.OutputFormat

const (
	SampleBalanced = engine.SampleBalanced
	SampleRandom   = engine.SampleRandom
	SamplePriority = engine.SamplePriority
)

const (
	OutputJSON      = engine.OutputJSON
	OutputMarkdown  = engine.OutputMarkdown
	OutputDirectory = engine.OutputDirectory
)

const (
	DefaultScrapeMaxPages      = engine.DefaultScrapeMaxPages
	DefaultScrapeMaxPerPattern = engine.DefaultScrapeMaxPerPattern
	DefaultScrapeTimeout       = engine.DefaultScrapeTimeout
)

var (
	ErrMissingBaseURL = engine.ErrMissingBaseURL
)

func ScrapeSite(ctx context.Context, opts *ScrapeOptions) (*ScrapeResult, error) {
	return engine.ScrapeSite(ctx, opts)
}

func RenderScrapeJSON(res *ScrapeResult) ([]byte, error) {
	return engine.RenderScrapeJSON(res)
}

func RenderScrapeMarkdown(res *ScrapeResult) string {
	return engine.RenderScrapeMarkdown(res)
}

func WriteScrapeDirectory(res *ScrapeResult, dir string) ([]string, error) {
	return engine.WriteScrapeDirectory(res, dir)
}
