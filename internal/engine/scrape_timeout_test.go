package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// slowNoSitemapServer serves a sitemap-less site whose pages each take
// pageDelay to respond, so an unbounded crawl would run for many seconds.
func slowNoSitemapServer(t *testing.T, pages int, pageDelay time.Duration) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" || strings.HasSuffix(r.URL.Path, ".xml") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/" {
			time.Sleep(pageDelay)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var b strings.Builder
		b.WriteString("<html><body><h1>page</h1>")
		for i := 0; i < pages; i++ {
			fmt.Fprintf(&b, `<a href="/p%d">p%d</a>`, i, i)
		}
		b.WriteString("</body></html>")
		_, _ = w.Write([]byte(b.String()))
	})
	return srv
}

func TestScrapeSiteTimeoutBoundsWallClock(t *testing.T) {
	srv := slowNoSitemapServer(t, 40, 300*time.Millisecond)

	timeout := 700 * time.Millisecond
	start := time.Now()
	res, err := ScrapeSite(context.Background(), &ScrapeOptions{
		BaseURL:  srv.URL,
		MaxPages: 30,
		Timeout:  timeout,
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("ScrapeSite should return partial results, got error: %v", err)
	}
	if res == nil {
		t.Fatal("expected a result")
	}
	// Unbounded, discovery alone would need 40 × 300ms = 12s. Allow generous
	// grace over the 700ms deadline to keep CI stable while still proving the
	// overall bound holds.
	if elapsed > 4*time.Second {
		t.Errorf("scrape ran %v, want ~%v (+grace): timeout is not an overall deadline", elapsed, timeout)
	}
}

func TestScrapeSiteZeroTimeoutStillCompletes(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" || strings.HasSuffix(r.URL.Path, ".xml") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><h1>home</h1><a href="/a">a</a></body></html>`))
	})

	res, err := ScrapeSite(context.Background(), &ScrapeOptions{
		BaseURL:  srv.URL,
		MaxPages: 3,
		Timeout:  0, // escape: no overall deadline; per-request default still applies
	})
	if err != nil {
		t.Fatalf("ScrapeSite: %v", err)
	}
	if res.Site.SampledPages == 0 {
		t.Error("expected at least one sampled page with Timeout=0")
	}
}
