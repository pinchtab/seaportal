package engine

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

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

func loadFixture(tb testing.TB, name string) string {
	tb.Helper()
	path := filepath.Join(repoTestdataDir(tb), filepath.FromSlash(name))
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

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
