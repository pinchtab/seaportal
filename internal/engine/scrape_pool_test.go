package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/seaportal/internal/engine/leakcheck"
)

func poolFixture(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<html><head><title>Page %s</title></head><body><h1>Hi</h1><p>%s body text here for extraction.</p></body></html>", r.URL.Path, r.URL.Path)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchAllOrderedAndComplete(t *testing.T) {
	leakcheck.CheckLeak(t)
	srv := poolFixture(t)
	urls := []string{srv.URL + "/", srv.URL + "/a", srv.URL + "/b", srv.URL + "/c", srv.URL + "/d"}

	got := fetchAll(context.Background(), urls, ScrapeOptions{BaseURL: srv.URL}, poolConfig{Concurrency: 3})

	if len(got) != len(urls) {
		t.Fatalf("got %d pages, want %d", len(got), len(urls))
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

func TestFetchAllRateLimitAcrossWorkers(t *testing.T) {
	srv := poolFixture(t)
	// Three same-host URLs with a 40ms per-host floor, three workers: the
	// shared limiter must serialize them → >= 2 intervals of spacing.
	urls := []string{srv.URL + "/1", srv.URL + "/2", srv.URL + "/3"}
	interval := 40 * time.Millisecond

	start := time.Now()
	got := fetchAll(context.Background(), urls, ScrapeOptions{BaseURL: srv.URL}, poolConfig{Concurrency: 3, MinInterval: interval})
	elapsed := time.Since(start)

	if len(got) != 3 {
		t.Fatalf("got %d pages, want 3", len(got))
	}
	if elapsed < 70*time.Millisecond {
		t.Errorf("elapsed %s, want >= ~2 intervals (rate limit not honored across workers)", elapsed)
	}
}

func TestFetchAllPartialFailure(t *testing.T) {
	leakcheck.CheckLeak(t)
	srv := poolFixture(t)
	// A bogus host fails DNS/connection; the good URLs must still succeed.
	urls := []string{srv.URL + "/", "http://nonexistent.invalid/x", srv.URL + "/ok"}

	got := fetchAll(context.Background(), urls, ScrapeOptions{BaseURL: srv.URL}, poolConfig{Concurrency: 2})

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

func TestFetchAllContextCancelled(t *testing.T) {
	leakcheck.CheckLeak(t)
	srv := poolFixture(t)
	var urls []string
	for i := 0; i < 6; i++ {
		urls = append(urls, fmt.Sprintf("%s/p%d", srv.URL, i))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled up front

	start := time.Now()
	got := fetchAll(ctx, urls, ScrapeOptions{BaseURL: srv.URL}, poolConfig{Concurrency: 3})
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
