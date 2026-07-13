package seaportal

import (
	"context"
	"net/http"
	"time"

	"github.com/pinchtab/seaportal/internal/engine"
)

type Result = engine.Result

var (
	ErrSecurityScheme     = engine.ErrSecurityScheme
	ErrSecurityDomain     = engine.ErrSecurityDomain
	ErrPrivateIPBlocked   = engine.ErrPrivateIPBlocked
	ErrSecurityResolve    = engine.ErrSecurityResolve
	ErrResponseTooLarge   = engine.ErrResponseTooLarge
	ErrDecompressTooLarge = engine.ErrDecompressTooLarge
	ErrBlockedByRobots    = engine.ErrBlockedByRobots
	ErrNeedsBrowser       = engine.ErrNeedsBrowser
)

type TransportInfo = engine.TransportInfo

type ResponseHeaders = engine.ResponseHeaders

type TraceInfo = engine.TraceInfo

type CDNInfo = engine.CDNInfo

type CacheAnalysis = engine.CacheAnalysis

type DedupeStats = engine.DedupeStats

type Options = engine.Options

const (
	DefaultClientTimeout     = engine.DefaultClientTimeout
	DefaultMaxRetryWait      = engine.DefaultMaxRetryWait
	DefaultTotalRetryTimeout = engine.DefaultTotalRetryTimeout
	DefaultRetryBackoffBase  = engine.DefaultRetryBackoffBase
	DefaultSitemapMaxDepth   = engine.DefaultSitemapMaxDepth
	DefaultSitemapMaxURLs    = engine.DefaultSitemapMaxURLs
	DefaultFeedMaxItems      = engine.DefaultFeedMaxItems
)

type SecurityPolicy = engine.SecurityPolicy

func DefaultSecurityPolicy() *SecurityPolicy {
	return engine.DefaultSecurityPolicy()
}

type LinkRetention = engine.LinkRetention

const (
	LinkRetentionAll    = engine.LinkRetentionAll
	LinkRetentionNone   = engine.LinkRetentionNone
	LinkRetentionText   = engine.LinkRetentionText
	LinkRetentionFooter = engine.LinkRetentionFooter
)

func ParseLinkRetention(s string) (LinkRetention, error) {
	return engine.ParseLinkRetention(s)
}

type Chunk = engine.Chunk

type ChunkConfig = engine.ChunkConfig

type ChunkStrategy = engine.ChunkStrategy

const (
	ChunkOff      = engine.ChunkOff
	ChunkHeading  = engine.ChunkHeading
	ChunkSentence = engine.ChunkSentence
	ChunkWindow   = engine.ChunkWindow
)

func ParseChunkConfig(s string) (ChunkConfig, error) {
	return engine.ParseChunkConfig(s)
}

func ChunkMarkdown(md string, cfg ChunkConfig) []Chunk {
	return engine.ChunkMarkdown(md, cfg)
}

type SplitConfig = engine.SplitConfig

type SplitFile = engine.SplitFile

func SplitResultToFiles(r Result, cfg SplitConfig) ([]SplitFile, error) {
	return engine.SplitResultToFiles(r, cfg)
}

type RankedSection = engine.RankedSection

func RankSections(content, query string, k1, b float64, topN int) []RankedSection {
	return engine.RankSections(content, query, k1, b, topN)
}

type PageProfile = engine.PageProfile

type PageClass = engine.PageClass

type ExtractionOutcome = engine.ExtractionOutcome

type BrowserDecision = engine.BrowserDecision

const (
	DecisionStaticHighConfidence = engine.DecisionStaticHighConfidence
	DecisionStaticOK             = engine.DecisionStaticOK
	DecisionStaticCaution        = engine.DecisionStaticCaution
	DecisionBrowserNeeded        = engine.DecisionBrowserNeeded
	DecisionBlocked              = engine.DecisionBlocked
	DecisionAuthRequired         = engine.DecisionAuthRequired
	DecisionUnreachable          = engine.DecisionUnreachable
	DecisionNotFound             = engine.DecisionNotFound
	DecisionUnsupported          = engine.DecisionUnsupported
)

type Validation = engine.Validation

type DedupeResult = engine.DedupeResult

type DedupeOptions = engine.DedupeOptions

type SnapshotOptions = engine.SnapshotOptions

type SnapshotNode = engine.SnapshotNode

type IndexPageResult = engine.IndexPageResult

type CardItem = engine.CardItem

func FromURL(targetURL string) Result {
	return engine.FromURL(targetURL)
}

func FromURLContext(ctx context.Context, targetURL string, opts Options) Result {
	return engine.FromURLContext(ctx, targetURL, opts)
}

func FromURLWithOptions(targetURL string, opts Options) Result {
	return engine.FromURLWithOptions(targetURL, opts)
}

func FromURLWithDedupe(targetURL string) Result {
	return engine.FromURLWithDedupe(targetURL)
}

func FromHTML(html string, targetURL string) Result {
	return engine.FromHTML(html, targetURL)
}

func FromHTMLWithOptions(html string, targetURL string, opts Options) Result {
	return engine.FromHTMLWithOptions(html, targetURL, opts)
}

func ResultToTEIXML(r Result) ([]byte, error) {
	return engine.ResultToTEIXML(r)
}

func FromResponse(resp *http.Response, targetURL string, start time.Time) Result {
	return engine.FromResponse(resp, targetURL, start)
}

func ExtractFromHTML(html string, targetURL string) (string, error) {
	return engine.ExtractFromHTML(html, targetURL)
}

func ClassifyPage(result Result) PageProfile {
	return engine.ClassifyPage(result)
}

func DetectSPA(html string) (signals []string, isSPA bool) {
	return engine.DetectSPA(html)
}

func DetectBlocked(html string) bool {
	return engine.DetectBlocked(html)
}

func QuickNeedsBrowser(html string) (needsBrowser bool, reason string) {
	return engine.QuickNeedsBrowser(html)
}

func Dedupe(content string) DedupeResult {
	return engine.Dedupe(content)
}

func DedupeWithOptions(content string, opts DedupeOptions) DedupeResult {
	return engine.DedupeWithOptions(content, opts)
}

func CleanupMarkdown(md string) string {
	return engine.CleanupMarkdown(md)
}

func PreprocessHTML(html string) string {
	return engine.PreprocessHTML(html)
}

func BuildSnapshot(htmlStr string) (*SnapshotNode, error) {
	return engine.BuildSnapshot(htmlStr)
}

func BuildSnapshotWithOptions(htmlStr string, opts SnapshotOptions) (*SnapshotNode, error) {
	return engine.BuildSnapshotWithOptions(htmlStr, opts)
}

type FetchBytesOptions = engine.FetchBytesOptions

func FetchBytes(ctx context.Context, rawURL string, opts FetchBytesOptions) ([]byte, http.Header, int, error) {
	return engine.FetchBytes(ctx, rawURL, opts)
}

func ValidateExtraction(r *Result) Validation {
	return engine.ValidateExtraction(r)
}

type SitemapEntry = engine.SitemapEntry

type FlattenSitemapOptions = engine.FlattenSitemapOptions

func FlattenSitemap(ctx context.Context, sitemapURL string, opts FlattenSitemapOptions) ([]SitemapEntry, error) {
	return engine.FlattenSitemap(ctx, sitemapURL, opts)
}

type FeedItem = engine.FeedItem

type ParseFeedOptions = engine.ParseFeedOptions

func ParseFeed(ctx context.Context, feedURL string, opts ParseFeedOptions) ([]FeedItem, error) {
	return engine.ParseFeed(ctx, feedURL, opts)
}

func SemanticFingerprint(content string) string {
	return engine.SemanticFingerprint(content)
}

func ContentChanged(oldContent, newContent string) bool {
	return engine.ContentChanged(oldContent, newContent)
}

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

var ErrMissingBaseURL = engine.ErrMissingBaseURL

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
