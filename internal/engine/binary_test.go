package engine

import "testing"

func TestIsBinaryContentType(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"", false},

		{"image/png", true},
		{"image/svg+xml", true},
		{"audio/mpeg", true},
		{"video/mp4", true},
		{"font/woff2", true},

		{"application/zip", true},
		{"application/gzip", true},
		{"application/x-tar", true},
		{"application/x-rar", true},
		{"application/octet-stream", true},
		{"application/x-msdownload", true},
		{"application/vnd.ms-excel", true},
		{"application/x-shockwave-flash", true},

		{"IMAGE/PNG", true},
		{"Application/OCTET-STREAM; charset=binary", true},
		{"  application/zip ; boundary=x", true},

		{"text/html", false},
		{"text/html; charset=utf-8", false},
		{"text/plain", false},
		{"application/json", false},
		{"application/ld+json", false},
		{"application/xml", false},
		{"application/atom+xml", false},
		{"application/xhtml+xml", false},
		{"application/javascript", false},

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

		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"Application/JSON", true},
		{"application/ld+json", true},
		{"application/vnd.api+json", true},

		{"application/xml", true},
		{"text/xml", true},
		{"application/atom+xml", true},
		{"application/rss+xml", true},
		{"image/svg+xml", true},

		{"application/xhtml+xml", false},

		{"text/plain", false},
		{"text/csv", false},
		{"text/html", false},

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
		{";charset=utf-8", ";charset=utf-8"},
	}
	for _, tt := range tests {
		if got := normalizeMediaType(tt.in); got != tt.want {
			t.Errorf("normalizeMediaType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
