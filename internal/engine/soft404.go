package engine

import (
	"fmt"
	"strings"
)

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

func detectSoft404(html string, contentLength int64) (bool, []string) {
	var hints []string

	if contentLength > 0 && contentLength < 500 {
		hints = append(hints, fmt.Sprintf("very-short-content:%d-bytes", contentLength))
	}

	htmlLower := strings.ToLower(html)
	for _, pattern := range soft404Patterns {
		if strings.Contains(htmlLower, pattern) {
			hints = append(hints, "error-text:"+strings.ReplaceAll(pattern, " ", "-"))
			break
		}
	}

	if titleStart := strings.Index(htmlLower, "<title>"); titleStart >= 0 {
		if titleEnd := strings.Index(htmlLower[titleStart:], "</title>"); titleEnd > 0 {
			title := htmlLower[titleStart : titleStart+titleEnd]
			if strings.Contains(title, "404") || strings.Contains(title, "not found") || strings.Contains(title, "error") {
				hints = append(hints, "error-title")
			}
		}
	}

	if len(hints) >= 2 {
		return true, hints
	}
	if len(hints) == 1 && strings.HasPrefix(hints[0], "very-short-content") {
		return false, hints
	}

	return false, hints
}
