package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// shellGroupsServer serves a sitemap-backed site with /docs/N and /blog/N
// sections plus a homepage, all fast and error-free.
func shellGroupsServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/robots.txt":
			http.NotFound(w, r)
		case r.URL.Path == "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			var b strings.Builder
			b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
			for i := 1; i <= 5; i++ {
				fmt.Fprintf(&b, "<url><loc>%s/docs/%d</loc></url>", srv.URL, i)
				fmt.Fprintf(&b, "<url><loc>%s/blog/%d</loc></url>", srv.URL, i)
			}
			b.WriteString("</urlset>")
			_, _ = w.Write([]byte(b.String()))
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><head><title>t</title></head><body><h1>t</h1><p>content here</p></body></html>`))
		}
	})
	return srv
}

func TestScrapeFilterDropsEmptyShellGroups(t *testing.T) {
	srv := shellGroupsServer(t)

	res, err := ScrapeSite(context.Background(), &ScrapeOptions{
		BaseURL:         srv.URL,
		MaxPages:        20,
		IncludePatterns: []string{"/docs/*"},
		Security:        allowInternalTestPolicy(), // httptest is loopback (T01)
	})
	if err != nil {
		t.Fatalf("ScrapeSite: %v", err)
	}
	if len(res.PageGroups) == 0 {
		t.Fatal("expected the /docs/* group to survive the filter")
	}
	for _, g := range res.PageGroups {
		if g.Sampled == 0 || len(g.Pages) == 0 {
			t.Errorf("empty-shell group %q leaked into pageGroups", g.Pattern)
		}
	}
	if res.Summary.UnsampledPatterns == 0 {
		t.Error("filtered-out patterns should be counted in summary.unsampledPatterns")
	}
}

func TestScrapeNoMatchFilterYieldsEmptyGroups(t *testing.T) {
	srv := shellGroupsServer(t)

	res, err := ScrapeSite(context.Background(), &ScrapeOptions{
		BaseURL:         srv.URL,
		MaxPages:        20,
		IncludePatterns: []string{"/nothing-matches/*"},
		Security:        allowInternalTestPolicy(), // httptest is loopback (T01)
	})
	if err != nil {
		t.Fatalf("ScrapeSite: %v", err)
	}
	if len(res.PageGroups) != 0 {
		t.Errorf("no-match filter must yield empty pageGroups, got %d groups", len(res.PageGroups))
	}
	if res.Summary.UnsampledPatterns == 0 {
		t.Error("summary.unsampledPatterns should report the skipped patterns")
	}
	found := false
	for _, r := range res.Summary.Recommendations {
		if strings.Contains(r, "0 of") && strings.Contains(r, "discovered URLs") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a self-explanatory no-match recommendation, got %v", res.Summary.Recommendations)
	}
}

func TestScrapeBudgetSkippedPatternsSummarized(t *testing.T) {
	srv := shellGroupsServer(t)

	res, err := ScrapeSite(context.Background(), &ScrapeOptions{
		BaseURL:  srv.URL,
		MaxPages: 2,
		Security: allowInternalTestPolicy(), // httptest is loopback (T01)
	})
	if err != nil {
		t.Fatalf("ScrapeSite: %v", err)
	}
	for _, g := range res.PageGroups {
		if g.Sampled == 0 {
			t.Errorf("budget-skipped group %q must not appear as an empty shell", g.Pattern)
		}
	}
}

func TestScrapeUnfilteredGroupsUnchanged(t *testing.T) {
	srv := shellGroupsServer(t)

	res, err := ScrapeSite(context.Background(), &ScrapeOptions{
		BaseURL:  srv.URL,
		MaxPages: 20,
		Security: allowInternalTestPolicy(), // httptest is loopback (T01)
	})
	if err != nil {
		t.Fatalf("ScrapeSite: %v", err)
	}
	if len(res.PageGroups) < 2 {
		t.Fatalf("expected docs and blog groups, got %v", res.PageGroups)
	}
	for _, g := range res.PageGroups {
		if g.Sampled == 0 {
			t.Errorf("unfiltered full-budget run should sample every group, got empty %q", g.Pattern)
		}
	}
	if res.Summary.UnsampledPatterns != 0 {
		t.Errorf("full coverage should report 0 unsampledPatterns, got %d", res.Summary.UnsampledPatterns)
	}
}
