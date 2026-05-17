// Package seaportal provides fast content extraction for AI agents.
// HTTP-first, no browser required.
//
// This is the public API. All implementation lives in internal/.
package seaportal

import (
	"net/http"
	"time"

	"github.com/pinchtab/seaportal/internal/engine"
)

// ── Types ───────────────────────────────────────────────────────────

// Result holds the extraction output for a URL.
type Result = engine.Result

// Options controls extraction behaviour.
type Options = engine.Options

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

// ── Extraction ──────────────────────────────────────────────────────

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

// ── Classification ──────────────────────────────────────────────────

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

// ── Content Processing ──────────────────────────────────────────────

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

// ── Snapshots ───────────────────────────────────────────────────────

// BuildSnapshot creates an accessibility tree from HTML.
func BuildSnapshot(htmlStr string) (*SnapshotNode, error) {
	return engine.BuildSnapshot(htmlStr)
}

// BuildSnapshotWithOptions creates an accessibility tree with custom options.
func BuildSnapshotWithOptions(htmlStr string, opts SnapshotOptions) (*SnapshotNode, error) {
	return engine.BuildSnapshotWithOptions(htmlStr, opts)
}

// ── Validation ──────────────────────────────────────────────────────

// ValidateExtraction assesses extraction quality.
func ValidateExtraction(r *Result) Validation {
	return engine.ValidateExtraction(r)
}

// ── Sitemap ─────────────────────────────────────────────────────────

// SitemapEntry is a single URL entry flattened from a sitemap.
type SitemapEntry = engine.SitemapEntry

// FlattenSitemapOptions controls FlattenSitemap behaviour.
type FlattenSitemapOptions = engine.FlattenSitemapOptions

// FlattenSitemap fetches a sitemap URL and recursively flattens
// `<sitemapindex>` references into a single slice of SitemapEntry.
var FlattenSitemap = engine.FlattenSitemap

// ── Feed (RSS / Atom / JSON Feed) ───────────────────────────────────

// FeedItem is a normalised feed entry across RSS 2.0, Atom 1.0, and
// JSON Feed 1.x sources.
type FeedItem = engine.FeedItem

// ParseFeedOptions controls ParseFeed behaviour.
type ParseFeedOptions = engine.ParseFeedOptions

// ParseFeed fetches a feed URL and parses it as RSS 2.0, Atom 1.0, or
// JSON Feed 1.x, returning a unified slice of FeedItem.
var ParseFeed = engine.ParseFeed

// ── Fingerprinting ──────────────────────────────────────────────────

// SemanticFingerprint generates a content fingerprint for change detection.
func SemanticFingerprint(content string) string {
	return engine.SemanticFingerprint(content)
}

// ContentChanged checks if content has changed based on fingerprints.
func ContentChanged(oldContent, newContent string) bool {
	return engine.ContentChanged(oldContent, newContent)
}
