package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/seaportal/internal/engine/leakcheck"
)

// poolFixture serves a robots.txt (with optional Crawl-delay) plus HTML pages
// for every other path.
func poolFixture(t *testing.T, robotsBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		if robotsBody == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, robotsBody)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, "<html><head><title>Page %s</title></head><body><h1>Hi</h1><p>%s body text here for extraction.</p></body></html>", r.URL.Path, r.URL.Path)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// runScrapeFetch drives fetchAndAssemble the way ScrapeSite does: normalized
// options, one shared robots cache + limiter per run.
func runScrapeFetch(ctx context.Context, t *testing.T, urls []string, baseURL string) ([]PageObject, []string) {
	t.Helper()
	o := ScrapeOptions{BaseURL: baseURL, Security: allowInternalTestPolicy()}.normalized()
	base, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse base %q: %v", baseURL, err)
	}
	robots := newCrawlDelayCacheWithFetch(FetchBytesOptions{Security: o.Security})
	return fetchAndAssemble(ctx, base, urls, o, robots, NewHostRateLimiter())
}

// Ported from the deleted fetchAll tests (T07): the shared pool must return
// one PageObject per input URL, in input order.
func TestFetchAndAssembleOrderedAndComplete(t *testing.T) {
	leakcheck.CheckLeak(t)
	srv := poolFixture(t, "")
	urls := []string{srv.URL + "/", srv.URL + "/a", srv.URL + "/b", srv.URL + "/c", srv.URL + "/d"}

	got, warnings := runScrapeFetch(context.Background(), t, urls, srv.URL)

	if len(got) != len(urls) {
		t.Fatalf("got %d pages, want %d", len(got), len(urls))
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	for i, p := range got {
		if p.URL != urls[i] {
			t.Errorf("page[%d].URL = %s, want %s (order not preserved)", i, p.URL, urls[i])
		}
		if p.Error != "" {
			t.Errorf("page[%d] unexpected error: %s", i, p.Error)
		}
		if p.Status != http.StatusOK {
			t.Errorf("page[%d].Status = %d, want 200", i, p.Status)
		}
	}
}

// Ported from the deleted fetchAll rate-limit test: a robots Crawl-delay must
// space same-host requests across pool workers via the shared limiter.
func TestFetchAndAssembleCrawlDelayAcrossWorkers(t *testing.T) {
	// Crawl-delay accepts fractions ("0.04" = 40ms); three same-host URLs on
	// three-plus workers must serialize into >= 2 intervals of spacing.
	srv := poolFixture(t, "User-agent: *\nCrawl-delay: 0.04\n")
	urls := []string{srv.URL + "/1", srv.URL + "/2", srv.URL + "/3"}

	start := time.Now()
	got, _ := runScrapeFetch(context.Background(), t, urls, srv.URL)
	elapsed := time.Since(start)

	if len(got) != 3 {
		t.Fatalf("got %d pages, want 3", len(got))
	}
	for i, p := range got {
		if p.Error != "" {
			t.Fatalf("page[%d] unexpected error: %s", i, p.Error)
		}
	}
	if elapsed < 70*time.Millisecond {
		t.Errorf("elapsed %s, want >= ~2 intervals (crawl-delay not honored across workers)", elapsed)
	}
}

// Ported from the deleted fetchAll partial-failure test: one bad host must not
// abort its siblings.
func TestFetchAndAssemblePartialFailure(t *testing.T) {
	leakcheck.CheckLeak(t)
	srv := poolFixture(t, "")
	// A bogus host fails DNS/connection; the good URLs must still succeed.
	urls := []string{srv.URL + "/", "http://nonexistent.invalid/x", srv.URL + "/ok"}

	got, _ := runScrapeFetch(context.Background(), t, urls, srv.URL)

	if len(got) != 3 {
		t.Fatalf("got %d pages, want 3", len(got))
	}
	if got[1].Error == "" {
		t.Errorf("page[1] (bad host) should carry an error, got none")
	}
	if got[0].Error != "" || got[0].Status != http.StatusOK {
		t.Errorf("page[0] should succeed despite sibling failure: err=%q status=%d", got[0].Error, got[0].Status)
	}
	if got[2].Error != "" || got[2].Status != http.StatusOK {
		t.Errorf("page[2] should succeed despite sibling failure: err=%q status=%d", got[2].Error, got[2].Status)
	}
}

// Ported from the deleted fetchAll cancellation test: a cancelled ctx yields
// one errored PageObject per URL, promptly.
func TestFetchAndAssembleContextCancelled(t *testing.T) {
	leakcheck.CheckLeak(t)
	srv := poolFixture(t, "")
	var urls []string
	for i := 0; i < 6; i++ {
		urls = append(urls, fmt.Sprintf("%s/p%d", srv.URL, i))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled up front

	start := time.Now()
	got, _ := runScrapeFetch(ctx, t, urls, srv.URL)
	elapsed := time.Since(start)

	if len(got) != len(urls) {
		t.Fatalf("got %d partial results, want %d", len(got), len(urls))
	}
	for i, p := range got {
		if p.Error == "" {
			t.Errorf("page[%d] should be cancelled/errored, got none", i)
		}
	}
	if elapsed > 2*time.Second {
		t.Errorf("cancellation not prompt: %s", elapsed)
	}
}

// T05: a hostile Crawl-delay is clamped and reported once per host. The run is
// bounded by a short pool timeout so the test never waits the clamped 30s.
func TestFetchAndAssembleClampWarning(t *testing.T) {
	srv := poolFixture(t, "User-agent: *\nCrawl-delay: 86400\n")
	urls := []string{srv.URL + "/a", srv.URL + "/b"}

	o := ScrapeOptions{BaseURL: srv.URL, Security: allowInternalTestPolicy(), Timeout: 300 * time.Millisecond}.normalized()
	base, _ := url.Parse(srv.URL)
	robots := newCrawlDelayCacheWithFetch(FetchBytesOptions{Security: o.Security})
	pages, warnings := fetchAndAssemble(context.Background(), base, urls, o, robots, NewHostRateLimiter())

	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one clamp notice for the host", warnings)
	}
	if !strings.Contains(warnings[0], "clamped") || !strings.Contains(warnings[0], "30s") {
		t.Errorf("warning = %q, want a clamped-to-30s notice", warnings[0])
	}
}
