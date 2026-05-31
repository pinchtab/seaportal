package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/pinchtab/seaportal"
)

// regression: snapshot-security-redirect-bypass
func TestFetchHTML_RespectsRedirectPolicy(t *testing.T) {
	var finalHit atomic.Bool
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		finalHit.Store(true)
		_, _ = fmt.Fprint(w, "<html><body><main>final</main></body></html>")
	})

	sec := &seaportal.SecurityPolicy{
		AllowedSchemes:      []string{"http"},
		MaxRedirects:        0,
		RevalidateRedirects: true,
	}

	_, err := fetchHTML(srv.URL+"/start", sec)
	if err != nil {
		t.Fatalf("fetchHTML: %v", err)
	}
	if finalHit.Load() {
		t.Fatalf("redirect target was fetched despite MaxRedirects=0")
	}
}
