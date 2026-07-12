package engine

import "testing"

func TestIsBinaryContentType(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		// Unknown/empty: proceed with extraction.
		{"", false},

		// Prefix families.
		{"image/png", true},
		{"image/svg+xml", true}, // image/ prefix wins even for +xml
		{"audio/mpeg", true},
		{"video/mp4", true},
		{"font/woff2", true},

		// Archives and executables.
		{"application/zip", true},
		{"application/gzip", true},
		{"application/x-tar", true},
		{"application/x-rar", true},
		{"application/octet-stream", true},
		{"application/x-msdownload", true},
		{"application/vnd.ms-excel", true}, // vnd.ms- prefix
		{"application/x-shockwave-flash", true},

		// Case-insensitive with parameters stripped.
		{"IMAGE/PNG", true},
		{"Application/OCTET-STREAM; charset=binary", true},
		{"  application/zip ; boundary=x", true},

		// Text-ish types flow through extraction.
		{"text/html", false},
		{"text/html; charset=utf-8", false},
		{"text/plain", false},
		{"application/json", false},
		{"application/ld+json", false},
		{"application/xml", false},
		{"application/atom+xml", false},
		{"application/xhtml+xml", false},
		{"application/javascript", false},

		// PDFs are deliberately NOT binary-skipped (extracted via ExtractPDFText).
		{"application/pdf", false},
	}

	for _, tt := range tests {
		if got := isBinaryContentType(tt.contentType); got != tt.want {
			t.Errorf("isBinaryContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
		}
	}
}

func TestIsRawTextContentType(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"", false},

		// JSON family: verbatim passthrough.
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"Application/JSON", true},
		{"application/ld+json", true},
		{"application/vnd.api+json", true},

		// XML family.
		{"application/xml", true},
		{"text/xml", true},
		{"application/atom+xml", true},
		{"application/rss+xml", true},
		{"image/svg+xml", true}, // +xml suffix (note: isBinaryContentType also claims this)

		// XHTML is HTML: keeps flowing through readability.
		{"application/xhtml+xml", false},

		// Plain text/CSV/HTML come out clean from the default path.
		{"text/plain", false},
		{"text/csv", false},
		{"text/html", false},

		// JSON-ish but not JSON.
		{"application/jsonx", false},
		{"text/json", false},
	}

	for _, tt := range tests {
		if got := isRawTextContentType(tt.contentType); got != tt.want {
			t.Errorf("isRawTextContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
		}
	}
}

func TestNormalizeMediaType(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Application/JSON; charset=utf-8", "application/json"},
		{"  TEXT/HTML  ", "text/html"},
		{"text/html;", "text/html"},
		{"", ""},
		{";charset=utf-8", ";charset=utf-8"}, // leading ';' (idx 0) is not stripped
	}
	for _, tt := range tests {
		if got := normalizeMediaType(tt.in); got != tt.want {
			t.Errorf("normalizeMediaType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
