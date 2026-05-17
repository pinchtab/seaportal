package engine

import "strings"

const charsPerToken = 4

// TruncateMarkdownAtParagraph returns md truncated to fit within an
// approximate maxTokens budget (using ~4 chars/token heuristic), cut at
// the latest paragraph boundary (blank-line separator) that fits. Falls
// back to single-newline boundary, then hard cut. Appends a "*[truncated]*"
// marker. Returns the truncated string and a bool indicating whether
// truncation occurred.
//
// Note: 4 chars/token is the cl100k-base approximation; we deliberately do
// not import a real tokenizer (heavy dep). For exact budgeting, callers
// should tokenize themselves.
//
// V1 limitation: paragraph boundaries can straddle fenced code blocks, so
// truncation inside a fence may leave an unterminated ```. Documented;
// V2 could honour fence balance.
func TruncateMarkdownAtParagraph(md string, maxTokens int) (string, bool) {
	if maxTokens <= 0 || md == "" {
		return md, false
	}
	budget := maxTokens * charsPerToken
	if len(md) <= budget {
		return md, false
	}
	// Latest "\n\n" boundary at or before budget.
	cut := strings.LastIndex(md[:budget], "\n\n")
	if cut < 0 {
		// Fall back to "\n".
		cut = strings.LastIndex(md[:budget], "\n")
	}
	if cut < 0 {
		// Hard cut.
		cut = budget
	}
	return md[:cut] + "\n\n*[truncated]*\n", true
}
