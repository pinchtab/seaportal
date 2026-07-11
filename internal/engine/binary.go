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
	ct := normalizeMediaType(contentType)
	for _, prefix := range binaryContentTypes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	return false
}

// normalizeMediaType lowercases a Content-Type and strips its parameters
// (e.g. "Application/JSON; charset=utf-8" -> "application/json").
func normalizeMediaType(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(ct, ";"); idx > 0 {
		ct = strings.TrimSpace(ct[:idx])
	}
	return ct
}

// isRawTextContentType reports whether a body is structured text (JSON/XML and
// their +json/+xml variants) that must pass through verbatim. The default
// extraction path converts HTML→markdown, which escapes markdown
// metacharacters (`_`, `[`, …) and corrupts such content — e.g. a JSON body's
// "node_id" becomes "node\_id", producing invalid JSON (ALP-036). text/plain
// and text/csv already emerge clean from the default path and are left alone;
// application/xhtml+xml is HTML and is deliberately excluded so it keeps
// flowing through readability.
func isRawTextContentType(contentType string) bool {
	ct := normalizeMediaType(contentType)
	if ct == "" || strings.Contains(ct, "xhtml") {
		return false
	}
	switch {
	case ct == "application/json" || strings.HasSuffix(ct, "+json"):
		return true
	case ct == "application/xml" || ct == "text/xml" || strings.HasSuffix(ct, "+xml"):
		return true
	default:
		return false
	}
}
