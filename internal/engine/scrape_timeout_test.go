package engine

import (
	"context"
	"errors"
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
		Security: allowInternalTestPolicy(), // httptest is loopback (T01)
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

// T21: cancelling the CALLER's context mid-fetch returns the partial result
// alongside ctx.Err() instead of dropping the output. (The internal --timeout
// budget elapsing stays a normal nil-error completion — asserted above.)
func TestScrapeSiteCallerCancelReturnsPartialResult(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			http.NotFound(w, r)
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			var b strings.Builder
			b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
			for i := 0; i < 8; i++ {
				fmt.Fprintf(&b, "<url><loc>%s/p%d</loc></url>", srv.URL, i)
			}
			b.WriteString("</urlset>")
			_, _ = w.Write([]byte(b.String()))
		default: // slow pages (longer than the cancel delay) so the cancel lands mid-fetch
			time.Sleep(400 * time.Millisecond)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><head><title>p</title></head><body><h1>p</h1><p>content</p></body></html>`))
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(120 * time.Millisecond) // discovery is fast; land mid-fetch
		cancel()
	}()

	res, err := ScrapeSite(ctx, &ScrapeOptions{
		BaseURL:  srv.URL,
		MaxPages: 8,
		Security: allowInternalTestPolicy(),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled alongside the partial result", err)
	}
	if res == nil {
		t.Fatal("partial result dropped on caller cancellation")
	}
	if len(res.Pages) == 0 {
		t.Fatal("expected one PageObject per sampled URL (partial results)")
	}
	var cancelled int
	for _, p := range res.Pages {
		if p.Error != "" {
			cancelled++
		}
	}
	if cancelled == 0 {
		t.Error("expected at least one page to carry the cancellation error")
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
		Timeout:  0,                         // escape: no overall deadline; per-request default still applies
		Security: allowInternalTestPolicy(), // httptest is loopback (T01)
	})
	if err != nil {
		t.Fatalf("ScrapeSite: %v", err)
	}
	if res.Site.SampledPages == 0 {
		t.Error("expected at least one sampled page with Timeout=0")
	}
}
