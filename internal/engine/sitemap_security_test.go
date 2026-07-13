package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestFlattenSitemap_SecurityPolicyStopsRedirects(t *testing.T) {
	var finalHit atomic.Bool
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/sitemap.xml", http.StatusFound)
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		finalHit.Store(true)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://example.com/a</loc></url></urlset>`))
	})

	_, err := FlattenSitemap(context.Background(), srv.URL+"/start", FlattenSitemapOptions{
		Client: srv.Client(),
		Security: &SecurityPolicy{
			AllowedSchemes:      []string{"http"},
			MaxRedirects:        0,
			RevalidateRedirects: true,
		},
	})
	if err == nil {
		t.Fatalf("expected redirect to be blocked")
	}
	if finalHit.Load() {
		t.Fatalf("redirect target was fetched despite MaxRedirects=0")
	}
}
