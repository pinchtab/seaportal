package engine

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"github.com/pinchtab/seaportal/internal/testserver/fixture"
)

func TestTransport_EdgeCases(t *testing.T) {
	const html = "<!doctype html><html><head><title>edge</title></head><body>" +
		"<article><h1>Edge</h1><p>Body content with enough words to register " +
		"as real content. Lorem ipsum dolor sit amet, consectetur adipiscing " +
		"elit, sed do eiusmod tempor incididunt ut labore et dolore magna.</p>" +
		"</article></body></html>"

	fastOpts := func(maxRetries int, maxRetryWait time.Duration) Options {
		return Options{
			MaxRetries:        maxRetries,
			MaxRetryWait:      maxRetryWait,
			TotalRetryTimeout: 2 * time.Second,
		}
	}

	t.Run("gzip", func(t *testing.T) {
		srv := fixture.New().Route("GET", "/page", fixture.Headers(
			http.Header{"Content-Encoding": []string{"gzip"}},
			fixture.Body(mustGzip([]byte(html)), "text/html; charset=utf-8"),
		))
		defer srv.Close()

		res := FromURLWithOptions(srv.URL()+"/page", fastOpts(0, time.Second))
		if res.Error != "" {
			t.Fatalf("error: %q", res.Error)
		}
		if !strings.Contains(res.Content, "Body content with enough words") {
			t.Fatalf("decompressed gzip body missing; content=%q", res.Content)
		}
	})

	t.Run("deflate", func(t *testing.T) {
		srv := fixture.New().Route("GET", "/page", fixture.Headers(
			http.Header{"Content-Encoding": []string{"deflate"}},
			fixture.Body(mustDeflate([]byte(html)), "text/html; charset=utf-8"),
		))
		defer srv.Close()

		res := FromURLWithOptions(srv.URL()+"/page", fastOpts(0, time.Second))
		if res.Error != "" {
			t.Fatalf("error: %q", res.Error)
		}
		if !strings.Contains(res.Content, "Body content with enough words") {
			t.Fatalf("decompressed deflate body missing; content=%q", res.Content)
		}
	})

	t.Run("brotli", func(t *testing.T) {
		srv := fixture.New().Route("GET", "/page", fixture.Headers(
			http.Header{"Content-Encoding": []string{"br"}},
			fixture.Body(mustBrotli([]byte(html)), "text/html; charset=utf-8"),
		))
		defer srv.Close()

		res := FromURLWithOptions(srv.URL()+"/page", fastOpts(0, time.Second))
		if res.Error != "" {
			t.Fatalf("error: %q", res.Error)
		}
		if !strings.Contains(res.Content, "Body content with enough words") {
			t.Fatalf("decompressed brotli body missing; content=%q", res.Content)
		}
	})

	t.Run("zstd", func(t *testing.T) {
		srv := fixture.New().Route("GET", "/page", fixture.Headers(
			http.Header{"Content-Encoding": []string{"zstd"}},
			fixture.Body(mustZstd([]byte(html)), "text/html; charset=utf-8"),
		))
		defer srv.Close()

		res := FromURLWithOptions(srv.URL()+"/page", fastOpts(0, time.Second))
		if res.Error != "" {
			t.Fatalf("error: %q", res.Error)
		}
		if !strings.Contains(res.Content, "Body content with enough words") {
			t.Fatalf("decompressed zstd body missing; content=%q", res.Content)
		}
	})

	t.Run("identity", func(t *testing.T) {
		srv := fixture.New().Route("GET", "/page",
			fixture.Body([]byte(html), "text/html; charset=utf-8"),
		)
		defer srv.Close()

		res := FromURLWithOptions(srv.URL()+"/page", fastOpts(0, time.Second))
		if res.Error != "" {
			t.Fatalf("error: %q", res.Error)
		}
		if !strings.Contains(res.Content, "Body content with enough words") {
			t.Fatalf("identity body missing; content=%q", res.Content)
		}
	})

	t.Run("unknown-encoding", func(t *testing.T) {
		srv := fixture.New().Route("GET", "/page", fixture.Headers(
			http.Header{"Content-Encoding": []string{"snappy"}},
			fixture.Body([]byte(html), "text/html; charset=utf-8"),
		))
		defer srv.Close()

		res := FromURLWithOptions(srv.URL()+"/page", fastOpts(0, time.Second))
		if res.Error != "" {
			t.Fatalf("error: %q", res.Error)
		}
		if !strings.Contains(res.Content, "Body content with enough words") {
			t.Fatalf("unknown-encoding body missing; content=%q", res.Content)
		}
	})

	t.Run("gzip-corrupt", func(t *testing.T) {
		srv := fixture.New().Route("GET", "/page", fixture.Headers(
			http.Header{"Content-Encoding": []string{"gzip"}},
			fixture.Body([]byte("this is definitely not gzip"), "text/html"),
		))
		defer srv.Close()

		res := FromURLWithOptions(srv.URL()+"/page", fastOpts(0, time.Second))
		if res.Error == "" {
			t.Fatalf("expected error surfacing corrupt gzip; got content=%q", res.Content)
		}
		errLower := strings.ToLower(res.Error)
		if !strings.Contains(errLower, "gzip") &&
			!strings.Contains(errLower, "invalid") &&
			!strings.Contains(errLower, "eof") {
			t.Fatalf("error %q doesn't look like a gzip decode failure", res.Error)
		}
	})

	t.Run("redirect-1hop", func(t *testing.T) {
		srv := fixture.New().
			Route("GET", "/start", fixture.Redirect("/final", http.StatusMovedPermanently)).
			Route("GET", "/final", fixture.Body([]byte(html), "text/html; charset=utf-8"))
		defer srv.Close()

		res := FromURLWithOptions(srv.URL()+"/start", fastOpts(0, time.Second))
		if res.Error != "" {
			t.Fatalf("error: %q", res.Error)
		}
		if !strings.Contains(res.Content, "Body content with enough words") {
			t.Fatalf("redirect target content missing; content=%q", res.Content)
		}
		if res.RedirectCount != 1 {
			t.Errorf("RedirectCount = %d, want 1", res.RedirectCount)
		}
		if !strings.HasSuffix(res.FinalURL, "/final") {
			t.Errorf("FinalURL = %q, want suffix /final", res.FinalURL)
		}
	})

	t.Run("redirect-loop-ceiling", func(t *testing.T) {
		srv := fixture.New().
			Route("GET", "/a", fixture.Redirect("/b", http.StatusMovedPermanently)).
			Route("GET", "/b", fixture.Redirect("/a", http.StatusMovedPermanently))
		defer srv.Close()

		res := FromURLWithOptions(srv.URL()+"/a", fastOpts(0, time.Second))
		if res.RedirectCount != 10 {
			t.Errorf("RedirectCount = %d, want 10 (cap from len(via)>=10)", res.RedirectCount)
		}
	})

	t.Run("dns-error", func(t *testing.T) {
		res := FromURLWithOptions(
			"http://this-host-does-not-exist.invalid/page",
			fastOpts(0, time.Second),
		)
		if res.Error == "" {
			t.Fatalf("expected DNS error, got empty Result.Error; status=%d", res.StatusCode)
		}
		errLower := strings.ToLower(res.Error)
		if !strings.Contains(errLower, "no such host") &&
			!strings.Contains(errLower, "lookup") &&
			!strings.Contains(errLower, "dns") {
			t.Fatalf("Result.Error %q doesn't look like a DNS failure", res.Error)
		}
	})

	t.Run("status-404", func(t *testing.T) {
		srv := fixture.New().Route("GET", "/missing", fixture.Status(http.StatusNotFound))
		defer srv.Close()

		res := FromURLWithOptions(srv.URL()+"/missing", fastOpts(0, time.Second))
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("StatusCode = %d, want 404", res.StatusCode)
		}
		if !containsReason(res.Profile.Reasons, "http-404-not-found") {
			t.Errorf("expected http-404-not-found reason, got %v", res.Profile.Reasons)
		}
		if res.IsBlocked {
			t.Errorf("404 should NOT set IsBlocked (page just missing, not blocked)")
		}
	})

	t.Run("status-500", func(t *testing.T) {
		srv := fixture.New().Route("GET", "/boom", fixture.Status(http.StatusInternalServerError))
		defer srv.Close()

		res := FromURLWithOptions(srv.URL()+"/boom", fastOpts(0, time.Second))
		if res.StatusCode != http.StatusInternalServerError {
			t.Errorf("StatusCode = %d, want 500", res.StatusCode)
		}
		if !containsReason(res.Profile.Reasons, "http-5xx-server-error") {
			t.Errorf("expected http-5xx-server-error reason, got %v", res.Profile.Reasons)
		}
	})

	t.Run("status-429-retry-after", func(t *testing.T) {
		hits := 0
		srv := fixture.New().Route("GET", "/limited", func(w http.ResponseWriter, _ *http.Request) {
			hits++
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		})
		defer srv.Close()

		start := time.Now()
		res := FromURLWithOptions(srv.URL()+"/limited", fastOpts(1, 100*time.Millisecond))
		elapsed := time.Since(start)

		if hits != 2 {
			t.Errorf("server hits = %d, want 2 (initial + 1 retry)", hits)
		}
		if res.RetryCount != 1 {
			t.Errorf("RetryCount = %d, want 1", res.RetryCount)
		}
		if res.StatusCode != http.StatusTooManyRequests {
			t.Errorf("StatusCode = %d, want 429", res.StatusCode)
		}
		foundReason := false
		for _, reason := range res.Profile.Reasons {
			if reason == "http-429-rate-limited" {
				foundReason = true
				break
			}
		}
		if !foundReason {
			t.Errorf("expected reason http-429-rate-limited, got %v", res.Profile.Reasons)
		}
		if elapsed > time.Second {
			t.Errorf("Retry-After:0 took %v; expected sub-second", elapsed)
		}
	})

	t.Run("data-url", func(t *testing.T) {
		res := FromURLWithOptions("data:text/html,<h1>hi</h1>", fastOpts(0, time.Second))
		if res.Error != "" {
			t.Fatalf("unexpected error: %q", res.Error)
		}
		if !strings.Contains(res.Content, "hi") {
			t.Fatalf("expected Content to contain 'hi'; got %q", res.Content)
		}
	})
}

func mustGzip(b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(b); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func mustDeflate(b []byte) []byte {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		panic(err)
	}
	if _, err := w.Write(b); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func mustBrotli(b []byte) []byte {
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	if _, err := w.Write(b); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func mustZstd(b []byte) []byte {
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		panic(err)
	}
	if _, err := w.Write(b); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
