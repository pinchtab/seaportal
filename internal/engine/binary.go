// Package portal provides content extraction with SPA detection
package engine

import "strings"

// binaryContentTypes lists MIME type prefixes for binary content that should be skipped.
//
// NOTE: "application/pdf" is intentionally NOT listed. PDFs are extracted via
// engine.ExtractPDFText and flow through the same Result.Content pipeline as
// HTML. Callers that genuinely want to skip PDFs can set Options.NoPDF (or
// pass --no-pdf), which restores the legacy "skipped binary content" behaviour
// for application/pdf responses.
var binaryContentTypes = []string{
	"image/",
	"audio/",
	"video/",
	"application/zip",
	"application/gzip",
	"application/x-tar",
	"application/x-rar",
	"application/octet-stream",
	"application/x-msdownload",
	"application/vnd.ms-",
	"application/x-shockwave-flash",
	"font/",
}

func isBinaryContentType(contentType string) bool {
	if contentType == "" {
		return false // Unknown content type, proceed with extraction
	}
	// Normalize: lowercase and strip parameters (e.g., "; charset=utf-8")
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(ct, ";"); idx > 0 {
		ct = strings.TrimSpace(ct[:idx])
	}
	for _, prefix := range binaryContentTypes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	return false
}
