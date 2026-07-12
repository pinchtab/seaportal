package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func urlset(locs ...string) string {
	s := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`
	for _, l := range locs {
		s += "<url><loc>" + l + "</loc></url>"
	}
	return s + `</urlset>`
}

func sitemapIndex(locs ...string) string {
	s := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`
	for _, l := range locs {
		s += "<sitemap><loc>" + l + "</loc></sitemap>"
	}
	return s + `</sitemapindex>`
}

func isXML(p string) bool { return len(p) >= 4 && p[len(p)-4:] == ".xml" }

// runDiscover starts an httptest server whose routes are built from its own
// base URL, runs discover against it, and returns the result plus the base URL
// so callers can strip it when comparing candidate paths. (ALP-002 unit
// coverage; the internal/testserver multi-page fixture is ALP-013/ALP-014.)
func runDiscover(t *testing.T, build func(base string) map[string]string, respectRobots *bool) (discoveryResult, string) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	for path, body := range build(srv.URL) {
		b, p := body, path
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case p == "/robots.txt":
				w.Header().Set("Content-Type", "text/plain")
			case isXML(p):
				w.Header().Set("Content-Type", "application/xml")
			default:
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
			}
			_, _ = w.Write([]byte(b))
		})
	}
	res, err := discover(context.Background(), ScrapeOptions{BaseURL: srv.URL, RespectRobots: respectRobots, Security: allowInternalTestPolicy()})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	return res, srv.URL
}

func candidatePaths(base string, urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		out = append(out, u[len(base):])
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDiscoverSitemapPresent(t *testing.T) {
	res, base := runDiscover(t, func(b string) map[string]string {
		return map[string]string{
			"/robots.txt":  "User-agent: *\nSitemap: " + b + "/sitemap.xml\n",
			"/sitemap.xml": urlset(b+"/", b+"/about", b+"/blog/1"),
		}
	}, nil)

	if !res.SitemapFound {
		t.Errorf("SitemapFound = false, want true")
	}
	if res.TotalURLsInSitemap != 3 {
		t.Errorf("TotalURLsInSitemap = %d, want 3", res.TotalURLsInSitemap)
	}
	if got, want := candidatePaths(base, res.URLs), []string{"/", "/about", "/blog/1"}; !equalStrings(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

func TestDiscoverSitemapIndex(t *testing.T) {
	res, base := runDiscover(t, func(b string) map[string]string {
		return map[string]string{
			"/robots.txt":    "Sitemap: " + b + "/sitemap.xml\n",
			"/sitemap.xml":   sitemapIndex(b+"/sitemap-a.xml", b+"/sitemap-b.xml"),
			"/sitemap-a.xml": urlset(b+"/a1", b+"/a2"),
			"/sitemap-b.xml": urlset(b + "/b1"),
		}
	}, nil)

	if !res.SitemapFound || res.TotalURLsInSitemap != 3 {
		t.Errorf("SitemapFound=%v total=%d, want true/3", res.SitemapFound, res.TotalURLsInSitemap)
	}
	if got, want := candidatePaths(base, res.URLs), []string{"/a1", "/a2", "/b1"}; !equalStrings(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

func TestDiscoverCrawlFallback(t *testing.T) {
	res, base := runDiscover(t, func(b string) map[string]string {
		return map[string]string{
			"/": `<html><body>` +
				`<a href="/a">a</a><a href="/b">b</a>` +
				`<a href="https://external.example/x">ext</a>` +
				`</body></html>`,
			"/a": `<html><body><a href="/c">c</a></body></html>`,
			"/b": `<html><body>b</body></html>`,
			"/c": `<html><body>c</body></html>`,
		}
	}, nil)

	if res.SitemapFound {
		t.Errorf("SitemapFound = true, want false (crawl fallback)")
	}
	if got, want := candidatePaths(base, res.URLs), []string{"/", "/a", "/b", "/c"}; !equalStrings(got, want) {
		t.Errorf("crawl candidates = %v, want %v (external excluded)", got, want)
	}
}

func TestDiscoverRobotsDisallow(t *testing.T) {
	routes := func(b string) map[string]string {
		return map[string]string{
			"/robots.txt":  "User-agent: *\nDisallow: /private\nSitemap: " + b + "/sitemap.xml\n",
			"/sitemap.xml": urlset(b+"/public", b+"/private/secret"),
		}
	}

	res, base := runDiscover(t, routes, boolPtr(true))
	if got := candidatePaths(base, res.URLs); !equalStrings(got, []string{"/public"}) {
		t.Errorf("with RespectRobots: candidates = %v, want [/public]", got)
	}

	res2, base2 := runDiscover(t, routes, boolPtr(false))
	if got := candidatePaths(base2, res2.URLs); !equalStrings(got, []string{"/private/secret", "/public"}) {
		t.Errorf("without RespectRobots: candidates = %v, want both", got)
	}
}
