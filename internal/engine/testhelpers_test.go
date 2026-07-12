package engine

// Shared test helpers (audit T35): fixture loading with repo-root testdata
// resolution and a page-map site server, replacing per-file copies of
// os.ReadFile("../../testdata/...") boilerplate and single-page
// httptest.NewServer handlers.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// repoTestdataDir resolves the repo-root testdata directory by walking up from
// the working directory — the one robust version of the cwd-guessing that
// internal/testserver/server.go does with hardcoded "..", "../.." probes.
func repoTestdataDir(tb testing.TB) string {
	tb.Helper()
	dir, err := os.Getwd()
	if err != nil {
		tb.Fatalf("getwd: %v", err)
	}
	for {
		cand := filepath.Join(dir, "testdata")
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			tb.Fatalf("testdata directory not found walking up from %s", dir)
		}
		dir = parent
	}
}

// loadFixture reads a fixture by its testdata-relative path (slash-separated,
// e.g. "static/wikipedia-latin-phrases.html") and fails the test on any error.
// Takes testing.TB so benchmarks can share it.
func loadFixture(tb testing.TB, name string) string {
	tb.Helper()
	path := filepath.Join(repoTestdataDir(tb), filepath.FromSlash(name))
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

// newSiteServer starts an httptest server that serves the given path→HTML map
// with sane defaults: text/html content type, and 404 for anything not in the
// map (including robots.txt and sitemap.xml, so extraction tests don't get
// surprise discovery behavior). Closed automatically via t.Cleanup.
func newSiteServer(tb testing.TB, pages map[string]string) *httptest.Server {
	tb.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	tb.Cleanup(srv.Close)
	return srv
}
