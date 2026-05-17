package engine

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newReq(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", DefaultAccept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", DefaultUserAgent)
	return req
}

func TestDiskCache_PutThenGet(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewDiskCache(dir, time.Hour)
	if err != nil {
		t.Fatalf("NewDiskCache: %v", err)
	}

	url := "https://example.com/foo"
	req := newReq(t, url)
	headers := http.Header{}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("X-Test", "hi")
	body := []byte("<html><body>hello</body></html>")

	if err := cache.Put(url, req, 200, headers, body); err != nil {
		t.Fatalf("Put: %v", err)
	}

	meta, got, ok := cache.Get(url, req)
	if !ok {
		t.Fatalf("Get: expected hit, got miss")
	}
	if meta.Status != 200 {
		t.Errorf("status: got %d want 200", meta.Status)
	}
	if meta.Headers.Get("X-Test") != "hi" {
		t.Errorf("headers: lost X-Test")
	}
	if string(got) != string(body) {
		t.Errorf("body: got %q want %q", got, body)
	}
}

func TestDiskCache_TTLExpiry(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewDiskCache(dir, time.Millisecond)
	if err != nil {
		t.Fatalf("NewDiskCache: %v", err)
	}
	url := "https://example.com/x"
	req := newReq(t, url)
	if err := cache.Put(url, req, 200, http.Header{}, []byte("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, _, ok := cache.Get(url, req); ok {
		t.Fatalf("Get: expected miss (TTL expired), got hit")
	}
}

func TestDiskCache_KeyVariesByURL(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Hour)

	r1 := newReq(t, "https://example.com/a")
	r2 := newReq(t, "https://example.com/b")
	k1 := cache.cacheKey("https://example.com/a", r1)
	k2 := cache.cacheKey("https://example.com/b", r2)
	if k1 == k2 {
		t.Fatalf("cacheKey collision: %s", k1)
	}

	_ = cache.Put("https://example.com/a", r1, 200, http.Header{}, []byte("A"))
	_ = cache.Put("https://example.com/b", r2, 200, http.Header{}, []byte("B"))

	entries, _ := os.ReadDir(dir)
	if len(entries) != 4 { // 2 headers + 2 bodies
		t.Fatalf("expected 4 cache files, got %d", len(entries))
	}
}

func TestDiskCache_KeyIgnoresIrrelevantHeaders(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Hour)
	url := "https://example.com/c"

	r1 := newReq(t, url)
	r1.Header.Set("Cache-Control", "no-cache")
	r2 := newReq(t, url)
	r2.Header.Set("Cache-Control", "max-age=60")
	r2.Header.Set("X-Trace-Id", "abc")

	if cache.cacheKey(url, r1) != cache.cacheKey(url, r2) {
		t.Fatalf("cacheKey should ignore Cache-Control/X-Trace-Id")
	}
}

func TestDiskCache_MissingHeadersFileIsMiss(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Hour)
	url := "https://example.com/d"
	req := newReq(t, url)

	key := cache.cacheKey(url, req)
	if err := os.WriteFile(filepath.Join(dir, key+".body.bin"), []byte("orphan"), 0644); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if _, _, ok := cache.Get(url, req); ok {
		t.Fatalf("Get: orphan body without headers should be a miss")
	}
}

// ── Integration ─────────────────────────────────────────────────────

func TestExtract_CacheHitSkipsNetwork(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body><h1>Hi</h1><p>world</p></body></html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Hour)

	// Pre-populate cache with a body matching what extract expects.
	req := newReq(t, srv.URL)
	headers := http.Header{}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	body := []byte("<html><body><h1>Cached</h1><p>from-cache</p></body></html>")
	if err := cache.Put(srv.URL, req, 200, headers, body); err != nil {
		t.Fatalf("Put: %v", err)
	}

	result := FromURLWithOptions(srv.URL, Options{CacheDir: dir, CacheTTL: time.Hour})
	if !result.CacheHit {
		t.Errorf("expected CacheHit=true")
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("expected 0 network hits, got %d", got)
	}
	if !strings.Contains(result.Content, "from-cache") {
		t.Errorf("expected cached body extraction, got content=%q", result.Content)
	}
}

func TestExtract_NoCacheBypassesReadsButWritesNew(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body><h1>Fresh</h1><p>from-network</p></body></html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Hour)
	req := newReq(t, srv.URL)
	stale := []byte("<html><body><h1>Stale</h1></body></html>")
	if err := cache.Put(srv.URL, req, 200, http.Header{"Content-Type": []string{"text/html"}}, stale); err != nil {
		t.Fatalf("Put: %v", err)
	}

	result := FromURLWithOptions(srv.URL, Options{CacheDir: dir, CacheTTL: time.Hour, NoCache: true})
	if result.CacheHit {
		t.Errorf("expected CacheHit=false under NoCache")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected 1 network hit, got %d", got)
	}

	// The cache write must have overwritten the stale entry.
	_, body, ok := cache.Get(srv.URL, req)
	if !ok {
		t.Fatalf("expected cache entry to still exist")
	}
	if string(body) == string(stale) {
		t.Errorf("expected cache to be refreshed, still stale: %q", body)
	}
}

// ── Re-validation unit tests ─────────────────────────────────────────

func TestDiskCache_GetStale_ReturnsStaleWithValidators(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Millisecond)
	url := "https://example.com/etag"
	req := newReq(t, url)
	headers := http.Header{}
	headers.Set("ETag", `"abc123"`)
	if err := cache.Put(url, req, 200, headers, []byte("body")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	meta, body, fresh, stale := cache.GetStale(url, req)
	if fresh {
		t.Fatalf("expected fresh=false")
	}
	if !stale {
		t.Fatalf("expected stale=true with ETag present")
	}
	if meta == nil || meta.Headers.Get("ETag") != `"abc123"` {
		t.Fatalf("expected meta with ETag, got %+v", meta)
	}
	if string(body) != "body" {
		t.Fatalf("body: got %q want %q", body, "body")
	}
}

func TestDiskCache_GetStale_NoValidatorsReturnsMiss(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Millisecond)
	url := "https://example.com/novalidators"
	req := newReq(t, url)
	if err := cache.Put(url, req, 200, http.Header{}, []byte("body")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	meta, body, fresh, stale := cache.GetStale(url, req)
	if fresh || stale {
		t.Fatalf("expected fresh=false stale=false, got fresh=%v stale=%v", fresh, stale)
	}
	if meta != nil || body != nil {
		t.Fatalf("expected nil meta+body on no-validator miss")
	}
}

func TestDiskCache_TouchByKey_UpdatesFetchedAt(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, 50*time.Millisecond)
	url := "https://example.com/touch"
	req := newReq(t, url)
	headers := http.Header{}
	headers.Set("ETag", `"v1"`)
	if err := cache.Put(url, req, 200, headers, []byte("body")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	if _, _, ok := cache.Get(url, req); ok {
		t.Fatalf("expected stale before Touch")
	}
	key := cache.cacheKey(url, req)
	if err := cache.TouchByKey(key); err != nil {
		t.Fatalf("TouchByKey: %v", err)
	}
	if _, _, ok := cache.Get(url, req); !ok {
		t.Fatalf("expected fresh after TouchByKey")
	}
}

func TestConditionalHeaders_BothPresent(t *testing.T) {
	h := http.Header{}
	h.Set("ETag", `"abc"`)
	h.Set("Last-Modified", "Wed, 21 Oct 2026 07:28:00 GMT")
	r := &cachedResponse{Headers: h}
	got := r.ConditionalHeaders()
	if got["If-None-Match"] != `"abc"` {
		t.Errorf("If-None-Match: got %q", got["If-None-Match"])
	}
	if got["If-Modified-Since"] != "Wed, 21 Oct 2026 07:28:00 GMT" {
		t.Errorf("If-Modified-Since: got %q", got["If-Modified-Since"])
	}
}

func TestConditionalHeaders_OnlyETag(t *testing.T) {
	h := http.Header{}
	h.Set("ETag", `"only"`)
	r := &cachedResponse{Headers: h}
	got := r.ConditionalHeaders()
	if len(got) != 1 || got["If-None-Match"] != `"only"` {
		t.Errorf("expected only If-None-Match, got %v", got)
	}
}

func TestConditionalHeaders_Neither(t *testing.T) {
	r := &cachedResponse{Headers: http.Header{}}
	got := r.ConditionalHeaders()
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// ── Re-validation integration tests ──────────────────────────────────

func TestExtract_RevalidationHit_304(t *testing.T) {
	const etag = `"revalidate-me"`
	var hits int32
	var sawConditional int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("If-None-Match") == etag {
			atomic.AddInt32(&sawConditional, 1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("ETag", etag)
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body><h1>Live</h1></body></html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	// Tiny TTL so the pre-populated entry is immediately stale.
	cache, _ := NewDiskCache(dir, time.Millisecond)
	req := newReq(t, srv.URL)
	headers := http.Header{}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("ETag", etag)
	cachedBody := []byte("<html><body><h1>Cached</h1><p>from-revalidated-cache</p></body></html>")
	if err := cache.Put(srv.URL, req, 200, headers, cachedBody); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	result := FromURLWithOptions(srv.URL, Options{CacheDir: dir, CacheTTL: time.Millisecond})
	if !result.CacheRevalidated {
		t.Errorf("expected CacheRevalidated=true")
	}
	if result.CacheHit {
		t.Errorf("expected CacheHit=false (network was hit)")
	}
	if atomic.LoadInt32(&sawConditional) != 1 {
		t.Errorf("expected server to see If-None-Match exactly once, got %d", sawConditional)
	}
	if !strings.Contains(result.Content, "from-revalidated-cache") {
		t.Errorf("expected cached body extraction, got %q", result.Content)
	}
	// FetchedAt should have been refreshed → a longer-TTL view sees it fresh.
	verify, _ := NewDiskCache(dir, time.Hour)
	if _, _, ok := verify.Get(srv.URL, req); !ok {
		t.Errorf("expected entry to be fresh after revalidation Touch")
	}
}

func TestExtract_RevalidationMiss_200(t *testing.T) {
	const etag = `"stale-tag"`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always serve fresh 200, even when conditional headers present.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("ETag", `"new-tag"`)
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body><h1>Fresh</h1><p>brand-new-body</p></body></html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Millisecond)
	req := newReq(t, srv.URL)
	headers := http.Header{}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("ETag", etag)
	staleBody := []byte("<html><body><h1>Old</h1><p>stale-body</p></body></html>")
	if err := cache.Put(srv.URL, req, 200, headers, staleBody); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	result := FromURLWithOptions(srv.URL, Options{CacheDir: dir, CacheTTL: time.Millisecond})
	if result.CacheRevalidated {
		t.Errorf("expected CacheRevalidated=false on 200")
	}
	if result.CacheHit {
		t.Errorf("expected CacheHit=false on 200")
	}
	if !strings.Contains(result.Content, "brand-new-body") {
		t.Errorf("expected fresh body, got %q", result.Content)
	}
	// Cache entry should have been replaced with the new body.
	verify, _ := NewDiskCache(dir, time.Hour)
	_, body, ok := verify.Get(srv.URL, req)
	if !ok {
		t.Fatalf("expected fresh cache entry after 200")
	}
	if !strings.Contains(string(body), "brand-new-body") {
		t.Errorf("expected cache replaced with new body, got %q", body)
	}
}

func TestExtract_NoValidatorsFallsThroughToFreshFetch(t *testing.T) {
	var sawConditional int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
			atomic.AddInt32(&sawConditional, 1)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body><h1>OK</h1><p>plain-fetch</p></body></html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Millisecond)
	req := newReq(t, srv.URL)
	// No ETag / Last-Modified on the cached entry.
	if err := cache.Put(srv.URL, req, 200, http.Header{"Content-Type": []string{"text/html"}}, []byte("<html><body>old</body></html>")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	result := FromURLWithOptions(srv.URL, Options{CacheDir: dir, CacheTTL: time.Millisecond})
	if result.CacheRevalidated {
		t.Errorf("expected CacheRevalidated=false (no validators)")
	}
	if atomic.LoadInt32(&sawConditional) != 0 {
		t.Errorf("expected no conditional headers sent, got %d", sawConditional)
	}
	if !strings.Contains(result.Content, "plain-fetch") {
		t.Errorf("expected fresh body extraction, got %q", result.Content)
	}
}

func TestExtract_OnlyCachesSuccessfulResponses(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("boom"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body><h1>OK</h1><p>good</p></body></html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Hour)
	req := newReq(t, srv.URL)

	// First call: 500 → must NOT be cached. Disable retries so the 500 surfaces.
	_ = FromURLWithOptions(srv.URL, Options{CacheDir: dir, CacheTTL: time.Hour, MaxRetries: 0})
	if _, _, ok := cache.Get(srv.URL, req); ok {
		t.Fatalf("500 response should not have been cached")
	}

	// Second call: 200 → should be cached.
	_ = FromURLWithOptions(srv.URL, Options{CacheDir: dir, CacheTTL: time.Hour, MaxRetries: 0})
	if _, _, ok := cache.Get(srv.URL, req); !ok {
		t.Fatalf("200 response should have been cached")
	}
}

// ── Stale-while-revalidate (SWR) unit tests ──────────────────────────

func TestDiskCache_GetStaleWithTolerance_FreshBand(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Hour)
	url := "https://example.com/swr-fresh"
	req := newReq(t, url)
	if err := cache.Put(url, req, 200, http.Header{}, []byte("body")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	meta, body, fresh, stale, beyond := cache.GetStaleWithTolerance(url, req, 10*time.Minute)
	if !fresh {
		t.Fatalf("expected fresh=true within TTL, got fresh=%v stale=%v beyond=%v", fresh, stale, beyond)
	}
	if stale || beyond {
		t.Fatalf("expected stale=false beyond=false, got stale=%v beyond=%v", stale, beyond)
	}
	if meta == nil || string(body) != "body" {
		t.Fatalf("expected meta+body, got meta=%v body=%q", meta, body)
	}
}

func TestDiskCache_GetStaleWithTolerance_SWRBand(t *testing.T) {
	dir := t.TempDir()
	// TTL tiny so the entry is immediately past-TTL; tolerance large enough
	// to keep it inside the SWR band for the test duration.
	cache, _ := NewDiskCache(dir, time.Millisecond)
	url := "https://example.com/swr-band"
	req := newReq(t, url)
	if err := cache.Put(url, req, 200, http.Header{}, []byte("body")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // now past TTL but still within tolerance

	meta, body, fresh, stale, beyond := cache.GetStaleWithTolerance(url, req, time.Hour)
	if fresh || beyond {
		t.Fatalf("expected fresh=false beyond=false, got fresh=%v beyond=%v", fresh, beyond)
	}
	if !stale {
		t.Fatalf("expected stale=true within SWR band")
	}
	if meta == nil || string(body) != "body" {
		t.Fatalf("expected meta+body, got meta=%v body=%q", meta, body)
	}
}

func TestDiskCache_GetStaleWithTolerance_BeyondBand(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Millisecond)
	url := "https://example.com/swr-beyond"
	req := newReq(t, url)
	headers := http.Header{}
	headers.Set("ETag", `"v1"`)
	if err := cache.Put(url, req, 200, headers, []byte("body")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	// Tolerance smaller than the time we slept → entry is beyond tolerance.
	meta, body, fresh, stale, beyond := cache.GetStaleWithTolerance(url, req, time.Millisecond)
	if fresh || stale {
		t.Fatalf("expected fresh=false stale=false, got fresh=%v stale=%v", fresh, stale)
	}
	if !beyond {
		t.Fatalf("expected beyondTolerance=true with validators present")
	}
	if meta == nil || meta.Headers.Get("ETag") != `"v1"` {
		t.Fatalf("expected meta with ETag, got %+v", meta)
	}
	if string(body) != "body" {
		t.Fatalf("body: got %q want %q", body, "body")
	}
}

func TestDiskCache_GetStaleWithTolerance_NoValidators(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Millisecond)
	url := "https://example.com/swr-no-validators"
	req := newReq(t, url)
	if err := cache.Put(url, req, 200, http.Header{}, []byte("body")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	meta, body, fresh, stale, beyond := cache.GetStaleWithTolerance(url, req, time.Millisecond)
	if fresh || stale || beyond {
		t.Fatalf("expected all false on past-tolerance no-validator entry, got fresh=%v stale=%v beyond=%v", fresh, stale, beyond)
	}
	if meta != nil || body != nil {
		t.Fatalf("expected nil meta+body on no-validator miss")
	}
}

// ── SWR integration tests ────────────────────────────────────────────

func TestExtract_SWRServesStaleAndRefreshesInBackground(t *testing.T) {
	const etag = `"swr-tag"`
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("ETag", etag)
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body><h1>NEW</h1><p>brand-new-body</p></body></html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Millisecond)
	req := newReq(t, srv.URL)
	headers := http.Header{}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("ETag", etag)
	cachedBody := []byte("<html><body><h1>OLD</h1><p>cached-stale-body</p></body></html>")
	if err := cache.Put(srv.URL, req, 200, headers, cachedBody); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // past TTL, inside tolerance below

	result := FromURLWithOptions(srv.URL, Options{
		CacheDir:            dir,
		CacheTTL:            time.Millisecond,
		CacheStaleTolerance: time.Hour,
	})
	if !result.CacheStale {
		t.Errorf("expected CacheStale=true, got %+v", result.CacheStale)
	}
	if result.CacheHit {
		t.Errorf("expected CacheHit=false for SWR replay")
	}
	if result.CacheRevalidated {
		t.Errorf("expected CacheRevalidated=false for SWR")
	}
	if !strings.Contains(result.Content, "cached-stale-body") {
		t.Errorf("expected stale cached body in content, got %q", result.Content)
	}

	// Background refresh should fire and replace cache. Poll briefly.
	deadline := time.Now().Add(2 * time.Second)
	verify, _ := NewDiskCache(dir, time.Hour)
	var got []byte
	for time.Now().Before(deadline) {
		_, body, ok := verify.Get(srv.URL, req)
		if ok && strings.Contains(string(body), "brand-new-body") {
			got = body
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got == nil {
		t.Fatalf("background refresh did not update cache within deadline; server hits=%d", atomic.LoadInt32(&hits))
	}
	if atomic.LoadInt32(&hits) < 1 {
		t.Errorf("expected at least one background hit, got %d", hits)
	}
}

func TestExtract_SWRBeyondToleranceFallsBackToSyncRevalidate(t *testing.T) {
	const etag = `"beyond-tag"`
	var sawConditional int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			atomic.AddInt32(&sawConditional, 1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("ETag", etag)
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body><h1>L</h1><p>live-body</p></body></html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cache, _ := NewDiskCache(dir, time.Millisecond)
	req := newReq(t, srv.URL)
	headers := http.Header{}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("ETag", etag)
	cachedBody := []byte("<html><body><h1>O</h1><p>cached-beyond-body</p></body></html>")
	if err := cache.Put(srv.URL, req, 200, headers, cachedBody); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(30 * time.Millisecond) // past TTL+tolerance below

	result := FromURLWithOptions(srv.URL, Options{
		CacheDir:            dir,
		CacheTTL:            time.Millisecond,
		CacheStaleTolerance: time.Millisecond, // tiny tolerance → past it
	})
	if result.CacheStale {
		t.Errorf("expected CacheStale=false beyond tolerance")
	}
	if !result.CacheRevalidated {
		t.Errorf("expected CacheRevalidated=true beyond tolerance (sync 304 replay)")
	}
	if atomic.LoadInt32(&sawConditional) != 1 {
		t.Errorf("expected server to receive exactly one conditional GET, got %d", sawConditional)
	}
	if !strings.Contains(result.Content, "cached-beyond-body") {
		t.Errorf("expected cached body replayed after 304, got %q", result.Content)
	}
}
