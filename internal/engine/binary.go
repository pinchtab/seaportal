package engine

import "strings"

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
		return false
	}
	ct := normalizeMediaType(contentType)
	for _, prefix := range binaryContentTypes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	return false
}

func normalizeMediaType(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(ct, ";"); idx > 0 {
		ct = strings.TrimSpace(ct[:idx])
	}
	return ct
}

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
