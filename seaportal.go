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

// ── Fingerprinting ──────────────────────────────────────────────────

// SemanticFingerprint generates a content fingerprint for change detection.
func SemanticFingerprint(content string) string {
	return engine.SemanticFingerprint(content)
}

// ContentChanged checks if content has changed based on fingerprints.
func ContentChanged(oldContent, newContent string) bool {
	return engine.ContentChanged(oldContent, newContent)
}
