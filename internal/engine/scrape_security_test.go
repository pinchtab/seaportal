package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func scrapeTestSite(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><head><title>Home</title></head><body><h1>Home</h1>` +
				`<a href="/about">About</a><a href="/secret">Secret</a></body></html>`))
		case "/sitemap.xml":
			http.NotFound(w, r)
		default:
			_, _ = w.Write([]byte(`<html><head><title>` + r.URL.Path + `</title></head><body><h1>` +
				r.URL.Path + `</h1><p>Some body content for extraction here.</p></body></html>`))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...)
	}
}

func TestScrapeSiteSecurityBlocksPrivateTargets(t *testing.T) {
	srv, requested := scrapeTestSite(t)

	res, err := ScrapeSite(context.Background(), &ScrapeOptions{
		BaseURL:  srv.URL,
		MaxPages: 10,
		Security: &SecurityPolicy{BlockPrivateIPs: true},
	})
	if err == nil {
		for _, p := range res.Pages {
			if p.Error == "" {
				t.Fatalf("page %s scraped despite BlockPrivateIPs", p.URL)
			}
		}
	}
	if got := requested(); len(got) != 0 {
		t.Errorf("crawl fetched %v despite BlockPrivateIPs", got)
	}
}

func TestScrapeSiteSecurityURLFilterVetoesEveryFetch(t *testing.T) {
	srv, requested := scrapeTestSite(t)

	var mu sync.Mutex
	var seen []string
	filter := func(_ context.Context, rawURL string) error {
		u, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		mu.Lock()
		seen = append(seen, u.Path)
		mu.Unlock()
		if strings.HasPrefix(u.Path, "/secret") {
			return fmt.Errorf("vetoed by host app: %s", u.Path)
		}
		return nil
	}

	res, err := ScrapeSite(context.Background(), &ScrapeOptions{
		BaseURL:  srv.URL,
		MaxPages: 10,
		Security: &SecurityPolicy{URLFilter: filter},
	})
	if err != nil {
		t.Fatalf("ScrapeSite: %v", err)
	}

	mu.Lock()
	filtered := map[string]bool{}
	for _, p := range seen {
		filtered[p] = true
	}
	mu.Unlock()
	for _, p := range requested() {
		if !filtered[p] {
			t.Errorf("path %s was fetched without passing the URLFilter", p)
		}
	}

	for _, got := range requested() {
		if strings.HasPrefix(got, "/secret") {
			t.Errorf("vetoed path %s was fetched anyway", got)
		}
	}
	for _, p := range res.Pages {
		if strings.Contains(p.URL, "/secret") && p.Error == "" {
			t.Errorf("vetoed page %s present without error", p.URL)
		}
	}
}

func TestValidateURLRunsURLFilterAfterBuiltInChecks(t *testing.T) {
	sentinel := errors.New("filter ran")
	calls := 0
	p := &SecurityPolicy{
		AllowedSchemes: []string{"https"},
		URLFilter: func(context.Context, string) error {
			calls++
			return sentinel
		},
	}
	if err := p.ValidateURL(context.Background(), "http://example.com/"); err == nil || errors.Is(err, sentinel) {
		t.Fatalf("scheme rejection expected before filter, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("filter ran despite scheme rejection")
	}
	if err := p.ValidateURL(context.Background(), "https://example.com/"); !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel from filter, got %v", err)
	}
}
