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

func TestScrapeOptionsDefaults(t *testing.T) {
	got := ScrapeOptions{BaseURL: "https://example.com"}.normalized()

	if got.MaxPages != DefaultScrapeMaxPages {
		t.Errorf("MaxPages = %d, want %d", got.MaxPages, DefaultScrapeMaxPages)
	}
	if got.MaxPerPattern != DefaultScrapeMaxPerPattern {
		t.Errorf("MaxPerPattern = %d, want %d", got.MaxPerPattern, DefaultScrapeMaxPerPattern)
	}
	if got.SampleStrategy != SampleBalanced {
		t.Errorf("SampleStrategy = %q, want %q", got.SampleStrategy, SampleBalanced)
	}
	if got.Output != OutputJSON {
		t.Errorf("Output = %q, want %q", got.Output, OutputJSON)
	}
	if got.Timeout != DefaultScrapeTimeout {
		t.Errorf("Timeout = %s, want %s", got.Timeout, DefaultScrapeTimeout)
	}
	if got.RespectRobots == nil || !*got.RespectRobots {
		t.Errorf("RespectRobots = %v, want default true", got.RespectRobots)
	}
	if got.UserAgent != DefaultUserAgent {
		t.Errorf("UserAgent = %q, want %q", got.UserAgent, DefaultUserAgent)
	}
}

func TestScrapeOptionsPreservesExplicitValues(t *testing.T) {
	no := false
	got := ScrapeOptions{
		BaseURL:        "https://example.com",
		MaxPages:       5,
		MaxPerPattern:  2,
		SampleStrategy: SampleRandom,
		Output:         OutputMarkdown,
		RespectRobots:  &no,
		UserAgent:      "custom-agent",
	}.normalized()

	if got.MaxPages != 5 || got.MaxPerPattern != 2 {
		t.Errorf("caps overwritten: MaxPages=%d MaxPerPattern=%d", got.MaxPages, got.MaxPerPattern)
	}
	if got.SampleStrategy != SampleRandom || got.Output != OutputMarkdown {
		t.Errorf("strategy/output overwritten: %q %q", got.SampleStrategy, got.Output)
	}
	if got.RespectRobots == nil || *got.RespectRobots {
		t.Errorf("explicit RespectRobots=false was not preserved: %v", got.RespectRobots)
	}
	if got.UserAgent != "custom-agent" {
		t.Errorf("UserAgent overwritten: %q", got.UserAgent)
	}
}

func TestScrapeSiteEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><head><title>Home</title></head><body><h1>Home</h1>` +
				`<a href="/about">About</a><a href="/blog/1">Post 1</a><a href="/blog/2">Post 2</a></body></html>`))
		case "/sitemap.xml":
			http.NotFound(w, r) // force crawl fallback
		default:
			w.Write([]byte(`<html><head><title>` + r.URL.Path + `</title></head><body><h1>` + r.URL.Path + `</h1><p>Some body content for extraction here.</p></body></html>`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := ScrapeSite(context.Background(), &ScrapeOptions{BaseURL: srv.URL, MaxPages: 10})
	if err != nil {
		t.Fatalf("ScrapeSite: %v", err)
	}
	if res.Site.BaseURL != srv.URL {
		t.Errorf("Site.BaseURL = %s, want %s", res.Site.BaseURL, srv.URL)
	}
	if res.Site.SitemapFound {
		t.Errorf("SitemapFound = true, want false (crawl fallback)")
	}
	if len(res.Pages) == 0 {
		t.Fatal("no pages scraped")
	}
	if res.Summary.ContentTypes == nil {
		t.Error("summary.ContentTypes must be non-nil")
	}
	for _, p := range res.Pages {
		if p.Status != http.StatusOK || p.Error != "" {
			t.Errorf("page %s: status=%d err=%q", p.URL, p.Status, p.Error)
		}
	}
}

func TestScrapeSiteValidatesBaseURL(t *testing.T) {
	if _, err := ScrapeSite(context.Background(), &ScrapeOptions{}); !errors.Is(err, ErrMissingBaseURL) {
		t.Fatalf("empty BaseURL err = %v, want ErrMissingBaseURL", err)
	}
	if _, err := ScrapeSite(context.Background(), nil); !errors.Is(err, ErrMissingBaseURL) {
		t.Fatalf("nil opts err = %v, want ErrMissingBaseURL", err)
	}
	// A supplied-but-malformed base URL (bare host, no scheme) is invalid, not
	// missing — distinct sentinel, and the message names the offending value.
	for _, bad := range []string{"example.com", "not-a-url"} {
		_, err := ScrapeSite(context.Background(), &ScrapeOptions{BaseURL: bad})
		if !errors.Is(err, ErrInvalidBaseURL) {
			t.Errorf("BaseURL %q err = %v, want ErrInvalidBaseURL", bad, err)
		}
		if errors.Is(err, ErrMissingBaseURL) {
			t.Errorf("BaseURL %q wrongly reported as missing: %v", bad, err)
		}
		if err != nil && !strings.Contains(err.Error(), bad) {
			t.Errorf("BaseURL %q err %q does not name the value", bad, err)
		}
	}
}

func TestDiscoveryBudgetReservesFetchWindow(t *testing.T) {
	if got := discoveryBudget(40 * time.Second); got != 20*time.Second {
		t.Errorf("discoveryBudget(40s) = %s, want 20s", got)
	}
	// Discovery must never claim the entire budget — some is always reserved.
	if got := discoveryBudget(time.Second); got >= time.Second {
		t.Errorf("discoveryBudget(1s) = %s, want < 1s (fetch reservation)", got)
	}
}

// ALP-040: crawl-fallback discovery must not consume the whole --timeout and
// leave the fetch phase with a dead context. This simulates slow discovery
// (the seed + section pages, fetched during the crawl to extract links, are
// slow) with fast leaf pages (only fetched in the fetch phase). With discovery
// sub-budgeted, at least some pages must come back fetched (not 100% canceled).
func TestScrapeSiteDiscoveryDoesNotStarveFetch(t *testing.T) {
	if testing.Short() {
		t.Skip("uses real sleeps to simulate slow discovery; runs in the full lane")
	}
	const slow = 30 * time.Millisecond
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/sitemap.xml" || r.URL.Path == "/robots.txt":
			http.NotFound(w, r) // force crawl fallback
		case r.URL.Path == "/":
			time.Sleep(slow) // slow seed (fetched during crawl)
			var b strings.Builder
			b.WriteString(`<html><body><h1>Home</h1>`)
			for i := 1; i <= 20; i++ {
				fmt.Fprintf(&b, `<a href="/s/%d">Section %d</a>`, i, i)
			}
			b.WriteString(`</body></html>`)
			_, _ = w.Write([]byte(b.String()))
		case strings.HasPrefix(r.URL.Path, "/s/"):
			time.Sleep(slow) // slow section pages (fetched during crawl for links)
			id := strings.TrimPrefix(r.URL.Path, "/s/")
			fmt.Fprintf(w, `<html><body><h1>Section</h1><a href="/p/%s">Leaf %s</a></body></html>`, id, id)
		default: // /p/* leaf pages: fast, only fetched in the fetch phase
			_, _ = w.Write([]byte(`<html><body><h1>Leaf</h1><p>Real leaf content worth extracting here.</p></body></html>`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	start := time.Now()
	res, err := ScrapeSite(context.Background(), &ScrapeOptions{
		BaseURL: srv.URL, MaxPages: 25, Timeout: 600 * time.Millisecond,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ScrapeSite: %v", err)
	}

	fetched := 0
	for _, p := range res.Pages {
		if p.Status == http.StatusOK && p.Error == "" {
			fetched++
		}
	}
	if fetched == 0 {
		t.Fatalf("0 of %d pages fetched — discovery starved the fetch phase", len(res.Pages))
	}
	// Wall-clock cap (ALP-029) still holds: don't run far past --timeout.
	if elapsed > 2*time.Second {
		t.Errorf("elapsed %s exceeds the timeout budget by too much", elapsed)
	}
}
