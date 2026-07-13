package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// A sitemap index whose children carry <lastmod> must not recurse into
// children older than opts.Since — and the old child must never be fetched at
// all (the whole point: a years-deep monthly archive stays cheap).
func TestFlattenSitemap_SinceSkipsOldIndexChildren(t *testing.T) {
	recent := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	old := time.Now().UTC().AddDate(0, 0, -400).Format(time.RFC3339)

	var mu sync.Mutex
	hits := map[string]int{}

	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits["/sitemap.xml"]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
			<sitemap><loc>%s/recent.xml</loc><lastmod>%s</lastmod></sitemap>
			<sitemap><loc>%s/old.xml</loc><lastmod>%s</lastmod></sitemap>
		</sitemapindex>`, base, recent, base, old)
	})
	mux.HandleFunc("/recent.xml", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits["/recent.xml"]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
			<url><loc>%s/a</loc><lastmod>%s</lastmod></url>
			<url><loc>%s/b</loc><lastmod>%s</lastmod></url>
		</urlset>`, base, recent, base, recent)
	})
	mux.HandleFunc("/old.xml", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits["/old.xml"]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
			<url><loc>%s/x</loc><lastmod>%s</lastmod></url>
		</urlset>`, base, old)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL

	cutoff := time.Now().AddDate(0, 0, -30)
	entries, err := FlattenSitemap(context.Background(), base+"/sitemap.xml", FlattenSitemapOptions{Since: cutoff})
	if err != nil {
		t.Fatalf("FlattenSitemap: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2 (only the recent child)", len(entries))
	}
	mu.Lock()
	defer mu.Unlock()
	if hits["/old.xml"] != 0 {
		t.Errorf("old child sitemap was fetched %d times; must be skipped entirely", hits["/old.xml"])
	}
	if hits["/recent.xml"] != 1 {
		t.Errorf("recent child fetched %d times, want 1", hits["/recent.xml"])
	}
}

// Without Since, nothing is filtered (backward compatible).
func TestFlattenSitemap_NoSinceKeepsAll(t *testing.T) {
	old := time.Now().UTC().AddDate(0, 0, -400).Format(time.RFC3339)
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
			<url><loc>%s/a</loc><lastmod>%s</lastmod></url>
			<url><loc>%s/b</loc></url>
		</urlset>`, base, old, base)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL

	entries, err := FlattenSitemap(context.Background(), base+"/sitemap.xml", FlattenSitemapOptions{})
	if err != nil {
		t.Fatalf("FlattenSitemap: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2 (no date filter)", len(entries))
	}
}

// A <url> with no <lastmod> is kept even under Since (fail-open — we don't drop
// content whose age we cannot determine).
func TestFlattenSitemap_SinceKeepsUndatedURLs(t *testing.T) {
	old := time.Now().UTC().AddDate(0, 0, -400).Format(time.RFC3339)
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
			<url><loc>%s/dated-old</loc><lastmod>%s</lastmod></url>
			<url><loc>%s/undated</loc></url>
		</urlset>`, base, old, base)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL

	entries, err := FlattenSitemap(context.Background(), base+"/sitemap.xml", FlattenSitemapOptions{Since: time.Now().AddDate(0, 0, -30)})
	if err != nil {
		t.Fatalf("FlattenSitemap: %v", err)
	}
	if len(entries) != 1 || entries[0].Loc != base+"/undated" {
		t.Errorf("got %v, want only the undated URL kept", entries)
	}
}

func TestParseSitemapTime(t *testing.T) {
	for _, tc := range []struct {
		in string
		ok bool
	}{
		{"2026-07-12T22:01:00Z", true},
		{"2026-07-12T22:01:00+02:00", true},
		{"2026-07-12", true},
		{"2026-07", true},
		{"2026", true},
		{"", false},
		{"not-a-date", false},
	} {
		_, ok := parseSitemapTime(tc.in)
		if ok != tc.ok {
			t.Errorf("parseSitemapTime(%q) ok=%v, want %v", tc.in, ok, tc.ok)
		}
	}
}
