package engine

import (
	"net/http"
	"reflect"
	"testing"
)

func TestResponseHeaders_Populate(t *testing.T) {
	hdr := http.Header{}
	set := map[string]string{
		"ETag":                      `"abc123"`,
		"Last-Modified":             "Wed, 21 Oct 2026 07:28:00 GMT",
		"Content-Encoding":          "gzip",
		"Server":                    "nginx/1.25",
		"Via":                       "1.1 varnish",
		"Age":                       "42",
		"Cache-Control":             "max-age=3600",
		"X-Cache":                   "HIT",
		"Vary":                      "Accept-Encoding",
		"CF-Cache-Status":           "DYNAMIC",
		"X-Served-By":               "cache-lhr7365-LHR",
		"X-Request-Id":              "req-1",
		"X-Correlation-Id":          "corr-1",
		"Traceparent":               "00-abc-def-01",
		"Tracestate":                "vendor=1",
		"Content-Security-Policy":   "default-src 'self'",
		"Strict-Transport-Security": "max-age=63072000",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "no-referrer",
		"Link":                      `<https://example.com/next>; rel="next"`,
		"X-Robots-Tag":              "noindex",
		"Content-Disposition":       `attachment; filename="a.pdf"`,
		"Content-Language":          "en-GB",
		"Transfer-Encoding":         "chunked",
		"X-Powered-By":              "PHP/8.3",
		"Server-Timing":             "db;dur=53",
	}
	for k, v := range set {
		hdr.Set(k, v)
	}
	hdr.Set("b3", "abc-def-1")

	var h ResponseHeaders
	h.populate(hdr)

	checks := []struct {
		field string
		got   string
		want  string
	}{
		{"ResponseETag", h.ResponseETag, `"abc123"`},
		{"ResponseLastModified", h.ResponseLastModified, "Wed, 21 Oct 2026 07:28:00 GMT"},
		{"ResponseContentEncoding", h.ResponseContentEncoding, "gzip"},
		{"ResponseServer", h.ResponseServer, "nginx/1.25"},
		{"ResponseVia", h.ResponseVia, "1.1 varnish"},
		{"ResponseAge", h.ResponseAge, "42"},
		{"ResponseCacheControl", h.ResponseCacheControl, "max-age=3600"},
		{"ResponseXCache", h.ResponseXCache, "HIT"},
		{"ResponseVary", h.ResponseVary, "Accept-Encoding"},
		{"ResponseCFCacheStatus", h.ResponseCFCacheStatus, "DYNAMIC"},
		{"ResponseXServedBy", h.ResponseXServedBy, "cache-lhr7365-LHR"},
		{"ResponseXRequestId", h.ResponseXRequestId, "req-1"},
		{"ResponseXCorrelationId", h.ResponseXCorrelationId, "corr-1"},
		{"ResponseTraceparent", h.ResponseTraceparent, "00-abc-def-01"},
		{"ResponseTracestate", h.ResponseTracestate, "vendor=1"},
		{"ResponseB3", h.ResponseB3, "abc-def-1"},
		{"ResponseCSP", h.ResponseCSP, "default-src 'self'"},
		{"ResponseHSTS", h.ResponseHSTS, "max-age=63072000"},
		{"ResponseXContentTypeOptions", h.ResponseXContentTypeOptions, "nosniff"},
		{"ResponseXFrameOptions", h.ResponseXFrameOptions, "DENY"},
		{"ResponseReferrerPolicy", h.ResponseReferrerPolicy, "no-referrer"},
		{"ResponseLink", h.ResponseLink, `<https://example.com/next>; rel="next"`},
		{"ResponseXRobotsTag", h.ResponseXRobotsTag, "noindex"},
		{"ResponseContentDisposition", h.ResponseContentDisposition, `attachment; filename="a.pdf"`},
		{"ResponseContentLanguage", h.ResponseContentLanguage, "en-GB"},
		{"ResponseTransferEncoding", h.ResponseTransferEncoding, "chunked"},
		{"ResponseXPoweredBy", h.ResponseXPoweredBy, "PHP/8.3"},
		{"ResponseServerTiming", h.ResponseServerTiming, "db;dur=53"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q want %q", c.field, c.got, c.want)
		}
	}
}

func TestResponseHeaders_Populate_CaseInsensitive(t *testing.T) {
	hdr := http.Header{}
	hdr.Set("eTaG", `"weird"`)
	hdr.Set("CACHE-CONTROL", "no-store")
	hdr.Set("x-request-id", "lower")

	var h ResponseHeaders
	h.populate(hdr)

	if h.ResponseETag != `"weird"` {
		t.Errorf("ResponseETag: got %q", h.ResponseETag)
	}
	if h.ResponseCacheControl != "no-store" {
		t.Errorf("ResponseCacheControl: got %q", h.ResponseCacheControl)
	}
	if h.ResponseXRequestId != "lower" {
		t.Errorf("ResponseXRequestId: got %q", h.ResponseXRequestId)
	}
}

func TestResponseHeaders_Populate_MultiValueKeepsFirst(t *testing.T) {
	hdr := http.Header{}
	hdr.Add("Link", `<https://example.com/style.css>; rel="preload"; as="style"`)
	hdr.Add("Link", `<https://example.com/next>; rel="next"`)
	hdr.Add("Vary", "Accept-Encoding")
	hdr.Add("Vary", "User-Agent")

	var h ResponseHeaders
	h.populate(hdr)

	if want := `<https://example.com/style.css>; rel="preload"; as="style"`; h.ResponseLink != want {
		t.Errorf("ResponseLink: got %q want first value %q", h.ResponseLink, want)
	}
	if h.ResponseVary != "Accept-Encoding" {
		t.Errorf("ResponseVary: got %q want first value only", h.ResponseVary)
	}
}

func TestResponseHeaders_Populate_SourceMapFallback(t *testing.T) {
	legacy := http.Header{}
	legacy.Set("X-SourceMap", "/legacy.map")
	var h ResponseHeaders
	h.populate(legacy)
	if h.ResponseSourceMap != "/legacy.map" {
		t.Errorf("legacy fallback: got %q want %q", h.ResponseSourceMap, "/legacy.map")
	}

	both := http.Header{}
	both.Set("SourceMap", "/modern.map")
	both.Set("X-SourceMap", "/legacy.map")
	var h2 ResponseHeaders
	h2.populate(both)
	if h2.ResponseSourceMap != "/modern.map" {
		t.Errorf("modern header must win: got %q", h2.ResponseSourceMap)
	}
}

func TestResponseHeaders_Populate_EmptyLeavesZeroValues(t *testing.T) {
	var h ResponseHeaders
	h.populate(http.Header{})
	if !reflect.DeepEqual(h, ResponseHeaders{}) {
		t.Errorf("expected zero-valued struct, got %+v", h)
	}
}

func TestPopulateResponseHeaders_ContentTypeOnResult(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Header.Set("ETag", `"e1"`)

	var result Result
	populateResponseHeaders(&result, resp)

	if result.ResponseContentType != "text/html; charset=utf-8" {
		t.Errorf("ResponseContentType: got %q", result.ResponseContentType)
	}
	if result.ResponseETag != `"e1"` {
		t.Errorf("ResponseETag: got %q", result.ResponseETag)
	}
}
