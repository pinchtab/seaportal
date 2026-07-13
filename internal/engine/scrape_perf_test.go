package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScrapeSitePopulatesTTFB(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" || strings.HasSuffix(r.URL.Path, ".xml") {
			http.NotFound(w, r)
			return
		}
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>home</title></head><body><h1>home</h1><p>content</p></body></html>`))
	})

	res, err := ScrapeSite(context.Background(), &ScrapeOptions{
		BaseURL:         srv.URL,
		MaxPages:        2,
		WithPerformance: true,
		Security:        allowInternalTestPolicy(),
	})
	if err != nil {
		t.Fatalf("ScrapeSite: %v", err)
	}
	if len(res.Pages) == 0 {
		t.Fatal("expected at least one page")
	}
	for _, p := range res.Pages {
		if p.Error != "" {
			continue
		}
		if p.Performance == nil {
			t.Fatalf("page %s: Performance is nil with WithPerformance", p.URL)
		}
		if p.Performance.TTFBMillis <= 0 {
			t.Errorf("page %s: ttfbMs = %d, want > 0 for a fetched page", p.URL, p.Performance.TTFBMillis)
		}
		if p.Performance.TotalBytes <= 0 {
			t.Errorf("page %s: totalBytes = %d, want > 0", p.URL, p.Performance.TotalBytes)
		}
		if p.Performance.Requests != 1 {
			t.Errorf("page %s: requests = %d, want 1", p.URL, p.Performance.Requests)
		}
	}
}
