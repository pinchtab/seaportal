// Package portal provides content extraction with SPA detection
package engine

import (
	"fmt"
	"strings"
)

// soft404Patterns are text patterns that suggest an error page despite 200 status.
var soft404Patterns = []string{
	"page not found",
	"404 not found",
	"404 error",
	"page doesn't exist",
	"page does not exist",
	"this page could not be found",
	"the requested page",
	"sorry, we couldn't find",
	"oops! we couldn't find",
	"we can't find that page",
	"no longer exists",
	"has been removed",
	"has been deleted",
	"content not available",
	"content unavailable",
}

// detectSoft404 checks if a page with 200 status is actually an error page.
// Returns (isSoft404, hints) where hints are the signals that triggered detection.
func detectSoft404(html string, contentLength int64) (bool, []string) {
	var hints []string

	// Check 1: Very short content (< 500 bytes) is suspicious
	if contentLength > 0 && contentLength < 500 {
		hints = append(hints, fmt.Sprintf("very-short-content:%d-bytes", contentLength))
	}

	// Check 2: Look for error page patterns in HTML
	htmlLower := strings.ToLower(html)
	for _, pattern := range soft404Patterns {
		if strings.Contains(htmlLower, pattern) {
			hints = append(hints, "error-text:"+strings.ReplaceAll(pattern, " ", "-"))
			break // One text match is enough
		}
	}

	// Check 3: Title tag contains 404 or "not found"
	if titleStart := strings.Index(htmlLower, "<title>"); titleStart >= 0 {
		if titleEnd := strings.Index(htmlLower[titleStart:], "</title>"); titleEnd > 0 {
			title := htmlLower[titleStart : titleStart+titleEnd]
			if strings.Contains(title, "404") || strings.Contains(title, "not found") || strings.Contains(title, "error") {
				hints = append(hints, "error-title")
			}
		}
	}

	// Determine if it's a soft-404: need at least 2 hints, or very short content + 1 other hint
	if len(hints) >= 2 {
		return true, hints
	}
	if len(hints) == 1 && strings.HasPrefix(hints[0], "very-short-content") {
		// Short content alone might be a minimalist page, need another signal
		return false, hints
	}

	return false, hints
}
