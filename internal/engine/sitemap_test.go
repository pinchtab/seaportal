package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFlattenSitemap_PlainURLSet(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/a</loc><lastmod>2025-01-01</lastmod></url>
  <url><loc>https://example.com/b</loc><changefreq>daily</changefreq></url>
  <url><loc>https://example.com/c</loc><priority>0.8</priority></url>
</urlset>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(xml))
	}))
	defer srv.Close()

	entries, err := FlattenSitemap(context.Background(), srv.URL+"/sitemap.xml", FlattenSitemapOptions{Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	if entries[0].Loc != "https://example.com/a" || entries[0].LastMod != "2025-01-01" {
		t.Errorf("entry[0] = %+v", entries[0])
	}
	if entries[1].ChangeFreq != "daily" {
		t.Errorf("entry[1].ChangeFreq = %q", entries[1].ChangeFreq)
	}
	if entries[2].Priority != "0.8" {
		t.Errorf("entry[2].Priority = %q", entries[2].Priority)
	}
}

func TestFlattenSitemap_NestedIndex(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/sitemap-index.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/sm-1.xml</loc></sitemap>
  <sitemap><loc>%s/sm-2.xml</loc></sitemap>
</sitemapindex>`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/sm-1.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://example.com/1</loc></url><url><loc>https://example.com/2</loc></url></urlset>`))
	})
	mux.HandleFunc("/sm-2.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://example.com/3</loc></url></urlset>`))
	})

	entries, err := FlattenSitemap(context.Background(), srv.URL+"/sitemap-index.xml", FlattenSitemapOptions{Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d: %v", len(entries), entries)
	}
	got := []string{entries[0].Loc, entries[1].Loc, entries[2].Loc}
	want := []string{"https://example.com/1", "https://example.com/2", "https://example.com/3"}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("entries[%d] = %q want %q", i, got[i], want[i])
		}
	}
}

func TestFlattenSitemap_HonorsMaxURLs(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&sb, `<url><loc>https://example.com/p%d</loc></url>`, i)
	}
	sb.WriteString(`</urlset>`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sb.String()))
	}))
	defer srv.Close()

	entries, err := FlattenSitemap(context.Background(), srv.URL+"/s.xml", FlattenSitemapOptions{MaxURLs: 10, Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(entries) != 10 {
		t.Fatalf("want 10, got %d", len(entries))
	}
}

func TestFlattenSitemap_HonorsMaxDepth(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// depth chain: /d0 -> /d1 -> /d2 -> /d3 (urlset)
	mux.HandleFunc("/d0", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><sitemapindex xmlns="x"><sitemap><loc>%s/d1</loc></sitemap></sitemapindex>`, srv.URL)
	})
	mux.HandleFunc("/d1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><sitemapindex xmlns="x"><sitemap><loc>%s/d2</loc></sitemap></sitemapindex>`, srv.URL)
	})
	mux.HandleFunc("/d2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><sitemapindex xmlns="x"><sitemap><loc>%s/d3</loc></sitemap></sitemapindex>`, srv.URL)
	})
	var d3Hits int32
	mux.HandleFunc("/d3", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&d3Hits, 1)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="x"><url><loc>https://example.com/leaf</loc></url></urlset>`))
	})

	entries, err := FlattenSitemap(context.Background(), srv.URL+"/d0", FlattenSitemapOptions{MaxDepth: 2, Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// MaxDepth=2: traverses d0(0) -> d1(1) -> d2(2); d2 references d3 at depth 3 which is > 2, so d3 must NOT be fetched.
	if atomic.LoadInt32(&d3Hits) != 0 {
		t.Errorf("d3 was fetched despite MaxDepth=2 (hits=%d)", d3Hits)
	}
	if len(entries) != 0 {
		t.Errorf("want 0 entries (depth cut off), got %d", len(entries))
	}
}

func TestFlattenSitemap_DedupesByLoc(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><sitemapindex xmlns="x"><sitemap><loc>%s/a</loc></sitemap><sitemap><loc>%s/b</loc></sitemap></sitemapindex>`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="x"><url><loc>https://example.com/dup</loc></url><url><loc>https://example.com/uniq-a</loc></url></urlset>`))
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="x"><url><loc>https://example.com/dup</loc></url><url><loc>https://example.com/uniq-b</loc></url></urlset>`))
	})

	entries, err := FlattenSitemap(context.Background(), srv.URL+"/index", FlattenSitemapOptions{Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries (dedup), got %d: %v", len(entries), entries)
	}
	seen := map[string]int{}
	for _, e := range entries {
		seen[e.Loc]++
	}
	if seen["https://example.com/dup"] != 1 {
		t.Errorf("dup not deduplicated: %v", seen)
	}
}

func TestFlattenSitemap_LoopProtection(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var aHits, bHits int32
	mux.HandleFunc("/A", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&aHits, 1)
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><sitemapindex xmlns="x"><sitemap><loc>%s/B</loc></sitemap></sitemapindex>`, srv.URL)
	})
	mux.HandleFunc("/B", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&bHits, 1)
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><sitemapindex xmlns="x"><sitemap><loc>%s/A</loc></sitemap></sitemapindex>`, srv.URL)
	})

	_, err := FlattenSitemap(context.Background(), srv.URL+"/A", FlattenSitemapOptions{MaxDepth: 20, Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if aHits != 1 || bHits != 1 {
		t.Errorf("loop protection failed: aHits=%d bHits=%d", aHits, bHits)
	}
}

func TestFlattenSitemap_GzipURL(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte(`<?xml version="1.0"?><urlset xmlns="x"><url><loc>https://example.com/gz1</loc></url><url><loc>https://example.com/gz2</loc></url></urlset>`))
	_ = gw.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	entries, err := FlattenSitemap(context.Background(), srv.URL+"/sitemap.xml.gz", FlattenSitemapOptions{Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(entries) != 2 || entries[0].Loc != "https://example.com/gz1" {
		t.Fatalf("want 2 gz entries, got %v", entries)
	}
}

func TestFlattenSitemap_HTTPRoundTrip(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/posts.xml</loc><lastmod>2025-03-01</lastmod></sitemap>
  <sitemap><loc>%s/pages.xml</loc></sitemap>
</sitemapindex>`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/posts.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://example.com/posts/1</loc><lastmod>2025-01-01</lastmod><changefreq>weekly</changefreq><priority>0.7</priority></url>
<url><loc>https://example.com/posts/2</loc></url>
</urlset>`))
	})
	mux.HandleFunc("/pages.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://example.com/about</loc></url>
</urlset>`))
	})

	entries, err := FlattenSitemap(context.Background(), srv.URL+"/sitemap.xml", FlattenSitemapOptions{Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	if entries[0].Priority != "0.7" || entries[0].ChangeFreq != "weekly" {
		t.Errorf("metadata not preserved: %+v", entries[0])
	}
}

func TestCLI_SitemapSubcommand(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://example.com/cli-1</loc></url><url><loc>https://example.com/cli-2</loc></url></urlset>`))
	})

	// Locate repo root (this file lives at internal/engine/sitemap_test.go).
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	bin := filepath.Join(t.TempDir(), "seaportal-cli-test")

	build := exec.Command("go", "build", "-o", bin, "./cmd/seaportal")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	defer func() { _ = os.Remove(bin) }()

	cmd := exec.Command(bin, "sitemap", srv.URL+"/sitemap.xml")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI failed: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "https://example.com/cli-1\nhttps://example.com/cli-2"
	if got != want {
		t.Errorf("CLI output mismatch:\nwant:\n%s\ngot:\n%s", want, got)
	}
}
