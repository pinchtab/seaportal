// Package engine on-disk content cache.
//
// DiskCache stores successful GET responses on the local filesystem keyed by a
// truncated SHA-256 of the request URL + a small set of representation-relevant
// headers (Accept, Accept-Language, User-Agent). Each entry is two files:
//
//	<key>.body.bin     — raw response bytes (still encoded if the server sent
//	                     Content-Encoding; replayed through the same decompress
//	                     path as a network hit).
//	<key>.headers.json — status + headers + fetched-at timestamp.
//
// Atomic write ordering: body first (rename), then headers (rename). A crashed
// Put may leave a body file behind with no headers file — Get treats that as a
// miss, so partial writes never surface stale or corrupt data. Errors during
// Put are non-fatal: callers log and proceed.
//
// Only 200 OK responses are cached. Errors, redirects, 4xx, and 5xx are never
// written.

package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DiskCache is a simple on-disk content cache keyed by URL + representation
// headers. Safe for concurrent use across processes (POSIX rename is atomic).
type DiskCache struct {
	dir string
	ttl time.Duration
}

// cachedResponse is the on-disk header/metadata sidecar for a cached body.
type cachedResponse struct {
	URL       string      `json:"url"`
	Status    int         `json:"status"`
	Headers   http.Header `json:"headers"`
	FetchedAt time.Time   `json:"fetchedAt"`
}

// NewDiskCache creates a cache rooted at dir with the given TTL. A zero TTL
// defaults to 24h. The directory is created if it does not exist.
func NewDiskCache(dir string, ttl time.Duration) (*DiskCache, error) {
	if dir == "" {
		return nil, errors.New("cache directory is empty")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &DiskCache{dir: dir, ttl: ttl}, nil
}

// cacheKey returns the 16-byte hex prefix of the SHA-256 of url + relevant
// request headers. Only Accept, Accept-Language and User-Agent participate —
// other headers (Cache-Control, cookies, traces) don't change representation.
func (c *DiskCache) cacheKey(url string, req *http.Request) string {
	h := sha256.New()
	h.Write([]byte(url))
	h.Write([]byte("\n"))
	if req != nil {
		h.Write([]byte(req.Header.Get("Accept")))
		h.Write([]byte("\n"))
		h.Write([]byte(req.Header.Get("Accept-Language")))
		h.Write([]byte("\n"))
		h.Write([]byte(req.Header.Get("User-Agent")))
	} else {
		h.Write([]byte("\n\n"))
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

func (c *DiskCache) headerPath(key string) string { return filepath.Join(c.dir, key+".headers.json") }
func (c *DiskCache) bodyPath(key string) string   { return filepath.Join(c.dir, key+".body.bin") }

// Get returns the cached response for (url, req) if present and still fresh.
// A missing headers file (whether the entry was never written or only the body
// half landed) is reported as a clean miss.
func (c *DiskCache) Get(url string, req *http.Request) (*cachedResponse, []byte, bool) {
	key := c.cacheKey(url, req)

	metaBytes, err := os.ReadFile(c.headerPath(key))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, false
		}
		return nil, nil, false
	}

	var meta cachedResponse
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, nil, false
	}

	if time.Since(meta.FetchedAt) > c.ttl {
		return nil, nil, false
	}

	body, err := os.ReadFile(c.bodyPath(key))
	if err != nil {
		return nil, nil, false
	}

	return &meta, body, true
}

// GetStale returns the cached response for (url, req) when present, either as
// fresh (within TTL) or as stale-but-validatable (past TTL with at least one
// of ETag/Last-Modified). Callers use the fresh bool to short-circuit the
// network, or use the stale bool plus ConditionalHeaders to send a conditional
// GET. A miss with neither validator returns (nil, nil, false, false).
//
// Equivalent to GetStaleWithTolerance(url, req, 0) where the SWR band has
// zero width and any past-TTL entry must either revalidate or miss.
func (c *DiskCache) GetStale(url string, req *http.Request) (*cachedResponse, []byte, bool, bool) {
	meta, body, fresh, _, beyond := c.GetStaleWithTolerance(url, req, 0)
	return meta, body, fresh, beyond
}

// GetStaleWithTolerance classifies a cached entry into one of three bands:
//
//   - fresh:           age <= TTL                            → serve directly.
//   - stale (SWR):     TTL < age <= TTL + tolerance          → serve as stale,
//     caller is expected to fire a background revalidate. Validators are NOT
//     required for the SWR band — within tolerance we trust the cached body
//     unconditionally.
//   - beyondTolerance: age > TTL + tolerance AND has validators (ETag /
//     Last-Modified) → caller runs a synchronous conditional GET.
//
// A past-TTL+tolerance entry without validators is a clean miss
// (nil, nil, false, false, false). A zero tolerance collapses the SWR band
// to zero width and the function behaves identically to GetStale.
func (c *DiskCache) GetStaleWithTolerance(url string, req *http.Request, tolerance time.Duration) (*cachedResponse, []byte, bool, bool, bool) {
	key := c.cacheKey(url, req)

	metaBytes, err := os.ReadFile(c.headerPath(key))
	if err != nil {
		return nil, nil, false, false, false
	}

	var meta cachedResponse
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, nil, false, false, false
	}

	body, err := os.ReadFile(c.bodyPath(key))
	if err != nil {
		return nil, nil, false, false, false
	}

	age := time.Since(meta.FetchedAt)
	if age <= c.ttl {
		return &meta, body, true, false, false
	}

	if tolerance > 0 && age <= c.ttl+tolerance {
		return &meta, body, false, true, false
	}

	etag := meta.Headers.Get("ETag")
	lastMod := meta.Headers.Get("Last-Modified")
	if etag == "" && lastMod == "" {
		return nil, nil, false, false, false
	}
	return &meta, body, false, false, true
}

// TouchByKey refreshes the FetchedAt timestamp on the headers sidecar for the
// given cache key. Called after a 304 confirms the cached body is still valid
// so that subsequent Gets see it as fresh. The body file is untouched.
func (c *DiskCache) TouchByKey(key string) error {
	metaBytes, err := os.ReadFile(c.headerPath(key))
	if err != nil {
		return err
	}
	var meta cachedResponse
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return err
	}
	meta.FetchedAt = time.Now()
	out, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return atomicWrite(c.headerPath(key), out)
}

// ConditionalHeaders returns the conditional-GET request headers derived from
// the cached response's validators. Empty map when neither validator is
// present.
func (r *cachedResponse) ConditionalHeaders() map[string]string {
	h := map[string]string{}
	if etag := r.Headers.Get("ETag"); etag != "" {
		h["If-None-Match"] = etag
	}
	if lm := r.Headers.Get("Last-Modified"); lm != "" {
		h["If-Modified-Since"] = lm
	}
	return h
}

// Put writes a cache entry atomically. Body lands first; headers second. A
// crash between the two writes leaves a body-only orphan which Get correctly
// treats as a miss. Callers must only Put for status == 200.
func (c *DiskCache) Put(url string, req *http.Request, status int, headers http.Header, body []byte) error {
	key := c.cacheKey(url, req)

	if err := atomicWrite(c.bodyPath(key), body); err != nil {
		return err
	}

	meta := cachedResponse{
		URL:       url,
		Status:    status,
		Headers:   headers,
		FetchedAt: time.Now(),
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := atomicWrite(c.headerPath(key), metaBytes); err != nil {
		return err
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
