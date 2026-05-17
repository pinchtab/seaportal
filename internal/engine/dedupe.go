// Package portal provides content extraction with SPA detection
package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math/bits"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/crypto/blake2b"
)

// Near-duplicate detection knobs. These are intentionally the only dials.
const (
	nearDupHammingThreshold = 3
	nearDupMinLength        = 40
	shingleSize             = 4
)

// DedupeResult holds deduplication metrics and output
type DedupeResult struct {
	Content              string   `json:"content,omitempty"`              // Deduplicated content
	OriginalBlocks       int      `json:"originalBlocks,omitempty"`       // Number of blocks before deduplication
	UniqueBlocks         int      `json:"uniqueBlocks,omitempty"`         // Number of unique blocks retained
	DuplicatesFound      int      `json:"duplicatesFound,omitempty"`      // Number of duplicate blocks removed (exact)
	DuplicateSignals     []string `json:"duplicateSignals,omitempty"`     // Types of duplicates detected (nav, heading, etc.)
	NearDuplicatesFound  int      `json:"nearDuplicatesFound,omitempty"`  // Number of near-duplicate blocks removed (simhash)
	NearDuplicateSignals []string `json:"nearDuplicateSignals,omitempty"` // Types of near-duplicates detected
}

// DedupeOptions configures deduplication behavior
type DedupeOptions struct {
	// MinBlockLen is the minimum length for a block to be tracked for deduplication
	// Shorter blocks (like single words) are always kept to avoid over-aggressive removal
	MinBlockLen int
	// NormalizeWhitespace collapses all whitespace to single spaces before comparison
	NormalizeWhitespace bool
	// CaseSensitive controls whether duplicate detection is case-sensitive
	CaseSensitive bool
	// NearDup enables simhash-based near-duplicate detection after the exact-hash
	// check misses. Only blocks whose normalised length is >= nearDupMinLength are
	// compared; matches within nearDupHammingThreshold bits are dropped.
	NearDup bool
}

// DefaultDedupeOptions returns sensible defaults for content deduplication
func DefaultDedupeOptions() DedupeOptions {
	return DedupeOptions{
		MinBlockLen:         20, // Ignore blocks under 20 chars
		NormalizeWhitespace: true,
		CaseSensitive:       false,
		NearDup:             true,
	}
}

// Dedupe removes duplicate blocks from markdown content.
// It splits content into logical blocks (paragraphs, headings, list items, etc.)
// and removes exact duplicates while preserving structure and order.
func Dedupe(content string) DedupeResult {
	return DedupeWithOptions(content, DefaultDedupeOptions())
}

func DedupeWithOptions(content string, opts DedupeOptions) DedupeResult {
	result := DedupeResult{
		Content: content,
	}

	if content == "" {
		return result
	}

	// Split into blocks (double newline separated, or heading-delimited)
	blocks := splitIntoBlocks(content)
	result.OriginalBlocks = len(blocks)

	if len(blocks) == 0 {
		return result
	}

	seen := make(map[string]bool)
	var signatures []uint64
	var uniqueBlocks []string
	var signals []string
	var nearSignals []string

	signalCounts := make(map[string]int)
	nearSignalCounts := make(map[string]int)

	for _, block := range blocks {
		trimmed := strings.TrimSpace(block)
		if trimmed == "" {
			// Preserve empty blocks for formatting
			uniqueBlocks = append(uniqueBlocks, block)
			continue
		}

		// Short blocks are always kept (navigation markers, etc.)
		// Exception: headings are always tracked regardless of length
		isHeading := strings.HasPrefix(trimmed, "#")
		if len(trimmed) < opts.MinBlockLen && !isHeading {
			uniqueBlocks = append(uniqueBlocks, block)
			continue
		}

		// Normalize for comparison
		normalized := normalizeBlock(trimmed, opts)
		hash := hashBlock(normalized)

		if seen[hash] {
			signal := classifyDuplicate(trimmed)
			signalCounts[signal]++
			result.DuplicatesFound++
			continue
		}
		seen[hash] = true

		// Near-duplicate pass — only for sufficiently long blocks. The exact-hash
		// path above remains primary; this catches templated boilerplate that
		// differs only by a date / counter / A/B-tested word.
		if opts.NearDup && len(normalized) >= nearDupMinLength {
			sig := simhash(normalized)
			matched := false
			for _, prev := range signatures {
				if hammingDistance(sig, prev) <= nearDupHammingThreshold {
					matched = true
					break
				}
			}
			if matched {
				signal := classifyDuplicate(trimmed)
				nearSignalCounts[signal]++
				result.NearDuplicatesFound++
				continue
			}
			signatures = append(signatures, sig)
		}

		uniqueBlocks = append(uniqueBlocks, block)
	}

	for signal, count := range signalCounts {
		if count > 0 {
			signals = append(signals, signal)
		}
	}
	for signal, count := range nearSignalCounts {
		if count > 0 {
			nearSignals = append(nearSignals, signal)
		}
	}

	result.UniqueBlocks = len(uniqueBlocks) - countEmptyBlocks(uniqueBlocks)
	result.Content = strings.Join(uniqueBlocks, "\n\n")
	result.DuplicateSignals = signals
	result.NearDuplicateSignals = nearSignals

	result.Content = cleanupWhitespace(result.Content)

	return result
}

func splitIntoBlocks(content string) []string {
	// Split on double newlines (paragraph boundaries)
	rawBlocks := strings.Split(content, "\n\n")

	var blocks []string
	for _, block := range rawBlocks {
		// Further split blocks that contain headings to isolate them
		parts := splitOnHeadings(block)
		blocks = append(blocks, parts...)
	}

	return blocks
}

func splitOnHeadings(block string) []string {
	headingRe := regexp.MustCompile(`(?m)^(#{1,6}\s+.+)$`)

	lines := strings.Split(block, "\n")
	if len(lines) <= 1 {
		return []string{block}
	}

	var result []string
	var current []string

	for _, line := range lines {
		if headingRe.MatchString(line) {
			if len(current) > 0 {
				result = append(result, strings.Join(current, "\n"))
				current = nil
			}
			result = append(result, line)
		} else {
			current = append(current, line)
		}
	}

	// Don't forget trailing content
	if len(current) > 0 {
		result = append(result, strings.Join(current, "\n"))
	}

	return result
}

func normalizeBlock(block string, opts DedupeOptions) string {
	s := block

	if !opts.CaseSensitive {
		s = strings.ToLower(s)
	}

	if opts.NormalizeWhitespace {
		wsRe := regexp.MustCompile(`\s+`)
		s = wsRe.ReplaceAllString(s, " ")
		s = strings.TrimSpace(s)
	}

	// Remove markdown formatting for comparison
	// This helps catch duplicates that differ only in formatting
	s = stripMarkdownFormatting(s)

	return s
}

func stripMarkdownFormatting(s string) string {
	s = regexp.MustCompile(`^#+\s*`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\*+`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`_+`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`(?m)^[\-\*\+]\s+`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?m)^\d+\.\s+`).ReplaceAllString(s, "")

	return s
}

func hashBlock(block string) string {
	h := sha256.Sum256([]byte(block))
	return hex.EncodeToString(h[:8]) // 16 hex chars is enough for dedup
}

func classifyDuplicate(block string) string {
	lower := strings.ToLower(block)

	navPatterns := []string{"home", "about", "contact", "menu", "navigation", "nav"}
	for _, p := range navPatterns {
		if strings.Contains(lower, p) {
			return "nav"
		}
	}

	if strings.HasPrefix(strings.TrimSpace(block), "#") {
		return "heading"
	}

	footerPatterns := []string{"copyright", "©", "all rights reserved", "privacy policy", "terms of"}
	for _, p := range footerPatterns {
		if strings.Contains(lower, p) {
			return "footer"
		}
	}

	relatedPatterns := []string{"related", "see also", "you might also", "recommended", "popular"}
	for _, p := range relatedPatterns {
		if strings.Contains(lower, p) {
			return "related"
		}
	}

	if strings.HasPrefix(strings.TrimSpace(block), "-") || strings.HasPrefix(strings.TrimSpace(block), "*") {
		return "card"
	}

	return "content"
}

func countEmptyBlocks(blocks []string) int {
	count := 0
	for _, b := range blocks {
		if strings.TrimSpace(b) == "" {
			count++
		}
	}
	return count
}

func cleanupWhitespace(content string) string {
	re := regexp.MustCompile(`\n{3,}`)
	content = re.ReplaceAllString(content, "\n\n")
	return strings.TrimSpace(content)
}

// DedupeLines removes exact duplicate lines from content.
// This is a lighter-weight deduplication for line-based content.
func DedupeLines(content string) string {
	lines := strings.Split(content, "\n")
	seen := make(map[string]bool)
	var unique []string

	for _, line := range lines {
		normalized := strings.TrimSpace(line)
		if normalized == "" {
			unique = append(unique, line)
			continue
		}

		// Use lowercase for comparison to catch case-insensitive duplicates
		key := strings.ToLower(normalized)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, line)
		}
	}

	return strings.Join(unique, "\n")
}

// NearDuplicateScore returns a similarity score (0-100) between two blocks
// 100 = identical, 0 = completely different
func NearDuplicateScore(a, b string) int {
	opts := DefaultDedupeOptions()
	normA := normalizeBlock(a, opts)
	normB := normalizeBlock(b, opts)

	if normA == normB {
		return 100
	}

	// Simple word overlap score
	wordsA := extractDedupeWords(normA)
	wordsB := extractDedupeWords(normB)

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	setA := make(map[string]bool)
	for _, w := range wordsA {
		setA[w] = true
	}

	overlap := 0
	for _, w := range wordsB {
		if setA[w] {
			overlap++
		}
	}

	// Jaccard-ish: overlap / union
	union := len(wordsA) + len(wordsB) - overlap
	if union == 0 {
		return 0
	}

	return (overlap * 100) / union
}

// simhash computes a 64-bit simhash signature for a normalised block, using
// 4-word shingles hashed with Blake2b. Returns 0 if the block has fewer than
// shingleSize tokens (caller should not compare such blocks).
func simhash(block string) uint64 {
	tokens := simhashTokens(block)
	if len(tokens) < shingleSize {
		return 0
	}

	var accum [64]int
	for i := 0; i+shingleSize <= len(tokens); i++ {
		shingle := strings.Join(tokens[i:i+shingleSize], " ")
		sum := blake2b.Sum256([]byte(shingle))
		// Fold 256 bits into 64 by XORing the four 64-bit lanes.
		h := binary.LittleEndian.Uint64(sum[0:8]) ^
			binary.LittleEndian.Uint64(sum[8:16]) ^
			binary.LittleEndian.Uint64(sum[16:24]) ^
			binary.LittleEndian.Uint64(sum[24:32])

		for b := 0; b < 64; b++ {
			if h&(uint64(1)<<b) != 0 {
				accum[b]++
			} else {
				accum[b]--
			}
		}
	}

	var sig uint64
	for b := 0; b < 64; b++ {
		if accum[b] > 0 {
			sig |= uint64(1) << b
		}
	}
	return sig
}

// simhashTokens splits a string on whitespace, lowercases each token, and
// strips non-alphanumeric runes. Empty tokens are skipped.
func simhashTokens(s string) []string {
	fields := strings.Fields(s)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		var b strings.Builder
		for _, r := range f {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(unicode.ToLower(r))
			}
		}
		if b.Len() > 0 {
			out = append(out, b.String())
		}
	}
	return out
}

// hammingDistance returns the number of differing bits between two uint64s.
func hammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

func extractDedupeWords(s string) []string {
	var words []string
	var current []rune

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, unicode.ToLower(r))
		} else if len(current) > 0 {
			words = append(words, string(current))
			current = nil
		}
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}

	return words
}
