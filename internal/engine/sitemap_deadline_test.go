package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// A sitemap index whose children are individually slow must abort at the
// deadline rather than flatten every child (ALP-051). The flatten now checks
// ctx mid-walk, and FlattenSitemap returns whatever it collected so far — a
// timed-out discovery keeps its partial URLs instead of throwing them away.
func TestFlattenSitemap_DeadlineBoundsAndKeepsPartial(t *testing.T) {
	const children = 10
	const perChildDelay = 150 * time.Millisecond // full walk ≈ 1.5s
	const budget = 300 * time.Millisecond

	var fetched int32
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/index.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
		for i := 0; i < children; i++ {
			_, _ = fmt.Fprintf(w, `<sitemap><loc>%s/child/%d</loc></sitemap>`, base, i)
		}
		_, _ = fmt.Fprint(w, `</sitemapindex>`)
	})
	mux.HandleFunc("/child/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetched, 1)
		select {
		case <-time.After(perChildDelay):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>%s%s/a</loc></url></urlset>`, base, r.URL.Path)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL

	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	start := time.Now()
	entries, err := FlattenSitemap(ctx, base+"/index.xml", FlattenSitemapOptions{})
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded (did the whole index flatten?)", err)
	}
	// Aborts near the budget, not after all 10 children (~1.5s).
	if elapsed > budget+600*time.Millisecond {
		t.Errorf("flatten ran %v, expected to abort near the %v budget", elapsed, budget)
	}
	// The URLs gathered before the deadline are preserved, not discarded.
	if len(entries) == 0 {
		t.Error("expected partial entries collected before the deadline")
	}
	// It did not fetch every child.
	if got := atomic.LoadInt32(&fetched); int(got) >= children {
		t.Errorf("fetched %d children; expected to stop well before all %d", got, children)
	}
	t.Logf("collected %d entries, fetched %d/%d children, in %v", len(entries), atomic.LoadInt32(&fetched), children, elapsed)
}
