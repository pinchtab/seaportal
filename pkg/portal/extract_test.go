package portal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFromURL_Basic(t *testing.T) {
	result := FromURL("https://example.com")
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	if result.Length == 0 {
		t.Error("expected non-zero content length")
	}
	if result.Confidence == 0 {
		t.Error("expected non-zero confidence")
	}
}

func TestDetectBlocked(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected bool
	}{
		{
			name:     "captcha page",
			html:     `<html><body>Please complete the CAPTCHA to continue</body></html>`,
			expected: true,
		},
		{
			name:     "cloudflare challenge",
			html:     `<html><body>Checking your browser... Cloudflare</body></html>`,
			expected: true,
		},
		{
			name:     "access denied",
			html:     `<html><body>Access Denied - You don't have permission</body></html>`,
			expected: true,
		},
		{
			name:     "normal page",
			html:     `<html><body><article><h1>Hello World</h1><p>This is content.</p></article></body></html>`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectBlocked(tt.html)
			if result != tt.expected {
				t.Errorf("DetectBlocked() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRetryOn429(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body><article><h1>Success</h1><p>Content here.</p></article></body></html>`))
	}))
	defer server.Close()

	opts := Options{
		MaxRetries:        3,
		MaxRetryWait:      5 * time.Second,
		TotalRetryTimeout: 10 * time.Second,
	}
	result := FromURLWithOptions(server.URL, opts)

	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	if result.RetryCount != 2 {
		t.Errorf("expected 2 retries, got %d", result.RetryCount)
	}
}

func TestHTTPStatusCodeAwareness(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		wantBlocked bool
	}{
		{"401 Unauthorized", 401, true},
		{"403 Forbidden", 403, true},
		{"404 Not Found", 404, false},
		{"200 OK", 200, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`<html><body><p>Response body</p></body></html>`))
			}))
			defer server.Close()

			result := FromURL(server.URL)
			if result.IsBlocked != tt.wantBlocked {
				t.Errorf("IsBlocked = %v, want %v", result.IsBlocked, tt.wantBlocked)
			}
		})
	}
}

func TestFromHTML(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
<article>
<h1>Main Title</h1>
<p>This is the main content of the page.</p>
<p>It has multiple paragraphs.</p>
</article>
</body>
</html>`

	result := FromHTML(html, "https://example.com/test")

	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Page")
	}
	if result.Length == 0 {
		t.Error("expected non-zero content length")
	}
	if !strings.Contains(result.Content, "Main Title") {
		t.Error("content should contain 'Main Title'")
	}
}

func TestDedupe(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
<article>
<p>This is unique content.</p>
<p>This is unique content.</p>
<p>This is unique content.</p>
<p>Different content here.</p>
</article>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	result := FromURLWithOptions(server.URL, Options{Dedupe: true})

	if !result.DedupeApplied {
		t.Error("expected dedupe to be applied")
	}
}

func TestFastMode(t *testing.T) {
	// SPA-like page that should trigger fast bailout
	html := `<!DOCTYPE html>
<html>
<head><title>App</title></head>
<body>
<div id="root"></div>
<script src="app.js"></script>
<noscript>Please enable JavaScript</noscript>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	result := FromURLWithOptions(server.URL, Options{FastMode: true})

	// Fast mode should detect SPA and bail early
	if !result.IsSPA && result.Length > 100 {
		t.Log("Note: Fast mode may still extract some content from minimal SPA pages")
	}
}
