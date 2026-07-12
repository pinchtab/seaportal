// Package seaportal provides fast content extraction for AI agents.
// HTTP-first, no browser required.
//
// This is the public API. All implementation lives in internal/.
package seaportal

import (
	"context"
	"net/http"
	"time"

	"github.com/pinchtab/seaportal/internal/engine"
)

// Result holds the extraction output for a URL.
//
// Failures are reported two ways: Result.Error is the JSON-serialized string,
// and Result.Err() returns the underlying error with its wrap chain intact —
// errors.Is / errors.As work against the exported sentinels (ErrPrivateIPBlocked,
// ErrBlockedByRobots, ErrResponseTooLarge, ErrNeedsBrowser, context.Canceled, …),
// so callers can branch on failure kind without string matching.
type Result = engine.Result

// Sentinel errors preserved on Result.Err(). Security sentinels are wrapped
// with target context at the block site; match with errors.Is.
var (
	// ErrSecurityScheme: URL scheme rejected by SecurityPolicy.AllowedSchemes.
	ErrSecurityScheme = engine.ErrSecurityScheme
	// ErrSecurityDomain: host rejected by the domain allow/deny lists.
	ErrSecurityDomain = engine.ErrSecurityDomain
	// ErrPrivateIPBlocked: target resolves to a private/internal IP (SSRF guard).
	ErrPrivateIPBlocked = engine.ErrPrivateIPBlocked
	// ErrSecurityResolve: target host could not be resolved for validation.
	ErrSecurityResolve = engine.ErrSecurityResolve
	// ErrResponseTooLarge: raw response body exceeded MaxResponseBytes.
	ErrResponseTooLarge = engine.ErrResponseTooLarge
	// ErrDecompressTooLarge: decompressed body exceeded MaxDecompressedBytes.
	ErrDecompressTooLarge = engine.ErrDecompressTooLarge
	// ErrBlockedByRobots: robots.txt disallows the target (RespectRobots set).
	ErrBlockedByRobots = engine.ErrBlockedByRobots
	// ErrNeedsBrowser: FastMode determined the page needs a real browser.
	ErrNeedsBrowser = engine.ErrNeedsBrowser
)

// Result's observability tail is grouped into anonymous embedded sub-structs;
// field promotion keeps flat access (r.TTFBMs, r.RetryCount, …) working and
// the JSON wire format is unchanged. The aliases below make the group types
// nameable through the facade.

// TransportInfo groups Result's retry/timing/redirect telemetry.
type TransportInfo = engine.TransportInfo

// ResponseHeaders groups Result's per-header response echoes.
type ResponseHeaders = engine.ResponseHeaders

// TraceInfo summarises distributed-tracing headers on the response.
type TraceInfo = engine.TraceInfo

// CDNInfo is the CDN/proxy-chain fingerprint derived from response headers.
type CDNInfo = engine.CDNInfo

// CacheAnalysis groups Result's cache-policy analysis fields.
type CacheAnalysis = engine.CacheAnalysis

// DedupeStats groups Result's block-deduplication statistics.
type DedupeStats = engine.DedupeStats

// Options controls extraction behaviour.
type Options = engine.Options

// SecurityPolicy is the opt-in SSRF / private-IP / redirect / decompression
// guard threaded through the fetch path. Set it on Options.Security. A nil
// policy keeps the historical unguarded behaviour.
type SecurityPolicy = engine.SecurityPolicy

// DefaultSecurityPolicy returns the recommended secure-by-default policy:
// block private/internal IPs, http/https only, a 10-redirect cap with per-hop
// revalidation, and 50 MiB raw / 200 MiB decompressed body caps.
func DefaultSecurityPolicy() *SecurityPolicy {
	return engine.DefaultSecurityPolicy()
}

// LinkRetention controls how inline Markdown links are kept in extracted output.
type LinkRetention = engine.LinkRetention

const (
	// LinkRetentionAll keeps inline `[text](url)` as-is (default).
	LinkRetentionAll = engine.LinkRetentionAll
	// LinkRetentionNone strips both link text and URL.
	LinkRetentionNone = engine.LinkRetentionNone
	// LinkRetentionText keeps the link text, drops the URL.
	LinkRetentionText = engine.LinkRetentionText
	// LinkRetentionFooter delegates to ConvertLinksToCitations:
	// numbered `⟨N⟩` markers and a `## References` section.
	LinkRetentionFooter = engine.LinkRetentionFooter
)

// ParseLinkRetention parses a mode name ("none"|"text"|"all"|"footer").
func ParseLinkRetention(s string) (LinkRetention, error) {
	return engine.ParseLinkRetention(s)
}

// Chunk is one piece of a chunked Markdown body.
type Chunk = engine.Chunk

// ChunkConfig controls Markdown chunking.
type ChunkConfig = engine.ChunkConfig

// ChunkStrategy selects a chunking algorithm.
type ChunkStrategy = engine.ChunkStrategy

const (
	// ChunkOff disables chunking (default).
	ChunkOff = engine.ChunkOff
	// ChunkHeading splits at H2/H3 boundaries.
	ChunkHeading = engine.ChunkHeading
	// ChunkSentence groups sentences to a token target.
	ChunkSentence = engine.ChunkSentence
	// ChunkWindow slides a char window with overlap.
	ChunkWindow = engine.ChunkWindow
)

// ParseChunkConfig parses the CLI form "heading" / "sentence[:N]" / "window[:N[:O]]".
func ParseChunkConfig(s string) (ChunkConfig, error) {
	return engine.ParseChunkConfig(s)
}

// ChunkMarkdown returns Markdown chunks under cfg, or nil when off / too short.
func ChunkMarkdown(md string, cfg ChunkConfig) []Chunk {
	return engine.ChunkMarkdown(md, cfg)
}

// SplitConfig controls SplitResultToFiles.
type SplitConfig = engine.SplitConfig

// SplitFile is one entry in the SplitResultToFiles manifest.
type SplitFile = engine.SplitFile

// SplitResultToFiles writes the Result's content split across multiple files
// under cfg.Dir and returns the manifest.
func SplitResultToFiles(r Result, cfg SplitConfig) ([]SplitFile, error) {
	return engine.SplitResultToFiles(r, cfg)
}

// RankedSection is a BM25-scored, heading-bounded slice of Markdown.
type RankedSection = engine.RankedSection

// RankSections scores Markdown sections (H2/H3-bounded) by BM25 against the
// query and returns them in descending score order. topN > 0 truncates;
// defaults k1=1.5, b=0.75 are applied when 0 is passed.
func RankSections(content, query string, k1, b float64, topN int) []RankedSection {
	return engine.RankSections(content, query, k1, b, topN)
}

// PageProfile describes the classification of a page.
type PageProfile = engine.PageProfile

// PageClass is the type of page (static, SSR, hydrated, dynamic, SPA, blocked).
type PageClass = engine.PageClass

// ExtractionOutcome indicates whether content is usable or needs a browser.
type ExtractionOutcome = engine.ExtractionOutcome

// BrowserDecision is the routing category exposed on Profile.Decision for
// callers (e.g. PinchTab) deciding whether to fall through to a real browser.
type BrowserDecision = engine.BrowserDecision

// Browser-routing decisions. See docs/reference/browser-discriminator.md.
const (
	DecisionStaticHighConfidence = engine.DecisionStaticHighConfidence
	DecisionStaticOK             = engine.DecisionStaticOK
	DecisionStaticCaution        = engine.DecisionStaticCaution
	DecisionBrowserNeeded        = engine.DecisionBrowserNeeded
	DecisionBlocked              = engine.DecisionBlocked
	DecisionUnreachable          = engine.DecisionUnreachable
	DecisionNotFound             = engine.DecisionNotFound
	DecisionUnsupported          = engine.DecisionUnsupported
)

// Validation holds extraction quality validation results.
type Validation = engine.Validation

// DedupeResult holds content deduplication metrics.
type DedupeResult = engine.DedupeResult

// DedupeOptions configures deduplication behaviour.
type DedupeOptions = engine.DedupeOptions

// SnapshotOptions controls accessibility snapshot generation.
type SnapshotOptions = engine.SnapshotOptions

// SnapshotNode is a node in the accessibility snapshot tree.
type SnapshotNode = engine.SnapshotNode

// IndexPageResult holds index/listing page extraction results.
type IndexPageResult = engine.IndexPageResult

// CardItem represents a card/item on an index page.
type CardItem = engine.CardItem

// FromURL extracts content from a URL with default options.
func FromURL(targetURL string) Result {
	return engine.FromURL(targetURL)
}

// FromURLWithOptions extracts content from a URL with custom options.
func FromURLWithOptions(targetURL string, opts Options) Result {
	return engine.FromURLWithOptions(targetURL, opts)
}

// FromURLWithDedupe extracts content with deduplication enabled.
func FromURLWithDedupe(targetURL string) Result {
	return engine.FromURLWithDedupe(targetURL)
}

// FromHTML extracts content from raw HTML.
func FromHTML(html string, targetURL string) Result {
	return engine.FromHTML(html, targetURL)
}

// FromHTMLWithOptions extracts content from raw HTML with custom options.
func FromHTMLWithOptions(html string, targetURL string, opts Options) Result {
	return engine.FromHTMLWithOptions(html, targetURL, opts)
}

// ResultToTEIXML wraps a Result into a TEI-Lite XML document.
func ResultToTEIXML(r Result) ([]byte, error) {
	return engine.ResultToTEIXML(r)
}

// FromResponse extracts content from an HTTP response.
func FromResponse(resp *http.Response, targetURL string, start time.Time) Result {
	return engine.FromResponse(resp, targetURL, start)
}

// ExtractFromHTML extracts markdown from raw HTML (simple interface).
func ExtractFromHTML(html string, targetURL string) (string, error) {
	return engine.ExtractFromHTML(html, targetURL)
}

// ClassifyPage determines the page type from extraction results.
func ClassifyPage(result Result) PageProfile {
	return engine.ClassifyPage(result)
}

// DetectSPA checks HTML for single-page application signals.
func DetectSPA(html string) (signals []string, isSPA bool) {
	return engine.DetectSPA(html)
}

// DetectBlocked checks if a page is blocked by bot protection.
func DetectBlocked(html string) bool {
	return engine.DetectBlocked(html)
}

// QuickNeedsBrowser checks if HTML likely needs a browser to render.
func QuickNeedsBrowser(html string) (needsBrowser bool, reason string) {
	return engine.QuickNeedsBrowser(html)
}

// Dedupe removes duplicate content blocks.
func Dedupe(content string) DedupeResult {
	return engine.Dedupe(content)
}

// DedupeWithOptions removes duplicate content blocks with custom options.
func DedupeWithOptions(content string, opts DedupeOptions) DedupeResult {
	return engine.DedupeWithOptions(content, opts)
}

// CleanupMarkdown normalises whitespace and formatting in markdown.
func CleanupMarkdown(md string) string {
	return engine.CleanupMarkdown(md)
}

// PreprocessHTML cleans HTML before extraction.
func PreprocessHTML(html string) string {
	return engine.PreprocessHTML(html)
}

// BuildSnapshot creates an accessibility tree from HTML.
func BuildSnapshot(htmlStr string) (*SnapshotNode, error) {
	return engine.BuildSnapshot(htmlStr)
}

// BuildSnapshotWithOptions creates an accessibility tree with custom options.
func BuildSnapshotWithOptions(htmlStr string, opts SnapshotOptions) (*SnapshotNode, error) {
	return engine.BuildSnapshotWithOptions(htmlStr, opts)
}

// FetchBytesOptions controls a raw network fetch with optional security checks.
type FetchBytesOptions = engine.FetchBytesOptions

// FetchBytes returns response bytes, headers, and status for rawURL.
func FetchBytes(ctx context.Context, rawURL string, opts FetchBytesOptions) ([]byte, http.Header, int, error) {
	return engine.FetchBytes(ctx, rawURL, opts)
}

// ValidateExtraction assesses extraction quality.
func ValidateExtraction(r *Result) Validation {
	return engine.ValidateExtraction(r)
}

// SitemapEntry is a single URL entry flattened from a sitemap.
type SitemapEntry = engine.SitemapEntry

// FlattenSitemapOptions controls FlattenSitemap behaviour.
type FlattenSitemapOptions = engine.FlattenSitemapOptions

// FlattenSitemap fetches a sitemap URL and recursively flattens
// `<sitemapindex>` references into a single slice of SitemapEntry.
func FlattenSitemap(ctx context.Context, sitemapURL string, opts FlattenSitemapOptions) ([]SitemapEntry, error) {
	return engine.FlattenSitemap(ctx, sitemapURL, opts)
}

// FeedItem is a normalised feed entry across RSS 2.0, Atom 1.0, and
// JSON Feed 1.x sources.
type FeedItem = engine.FeedItem

// ParseFeedOptions controls ParseFeed behaviour.
type ParseFeedOptions = engine.ParseFeedOptions

// ParseFeed fetches a feed URL and parses it as RSS 2.0, Atom 1.0, or
// JSON Feed 1.x, returning a unified slice of FeedItem.
func ParseFeed(ctx context.Context, feedURL string, opts ParseFeedOptions) ([]FeedItem, error) {
	return engine.ParseFeed(ctx, feedURL, opts)
}

// SemanticFingerprint generates a content fingerprint for change detection.
func SemanticFingerprint(content string) string {
	return engine.SemanticFingerprint(content)
}

// ContentChanged checks if content has changed based on fingerprints.
func ContentChanged(oldContent, newContent string) bool {
	return engine.ContentChanged(oldContent, newContent)
}
