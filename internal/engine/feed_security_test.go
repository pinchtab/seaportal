package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// regression: parse-feed-security-redirect-bypass
func TestParseFeed_SecurityPolicyStopsRedirects(t *testing.T) {
	var finalHit atomic.Bool
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/feed.xml", http.StatusFound)
	})
	mux.HandleFunc("/feed.xml", func(w http.ResponseWriter, r *http.Request) {
		finalHit.Store(true)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><item><title>x</title></item></channel></rss>`))
	})

	_, err := ParseFeed(context.Background(), srv.URL+"/start", ParseFeedOptions{
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
