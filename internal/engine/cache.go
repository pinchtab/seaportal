package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type DiskCache struct {
	dir string
	ttl time.Duration

	now func() time.Time
}

type cachedResponse struct {
	URL       string      `json:"url"`
	Status    int         `json:"status"`
	Headers   http.Header `json:"headers"`
	FetchedAt time.Time   `json:"fetchedAt"`
}

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
	return &DiskCache{dir: dir, ttl: ttl, now: time.Now}, nil
}

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

	if c.now().Sub(meta.FetchedAt) > c.ttl {
		return nil, nil, false
	}

	body, err := os.ReadFile(c.bodyPath(key))
	if err != nil {
		return nil, nil, false
	}

	return &meta, body, true
}

func (c *DiskCache) GetStale(url string, req *http.Request) (*cachedResponse, []byte, bool, bool) {
	meta, body, fresh, _, beyond := c.GetStaleWithTolerance(url, req, 0)
	return meta, body, fresh, beyond
}

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

	age := c.now().Sub(meta.FetchedAt)
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

func (c *DiskCache) TouchByKey(key string) error {
	metaBytes, err := os.ReadFile(c.headerPath(key))
	if err != nil {
		return err
	}
	var meta cachedResponse
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return err
	}
	meta.FetchedAt = c.now()
	out, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return atomicWrite(c.headerPath(key), out)
}

func (r *cachedResponse) toHTTPResponse(body []byte, req *http.Request) *http.Response {
	return &http.Response{
		Status:     fmt.Sprintf("%d %s", r.Status, http.StatusText(r.Status)),
		StatusCode: r.Status,
		Header:     r.Headers.Clone(),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}
}

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

func (c *DiskCache) Put(url string, req *http.Request, status int, headers http.Header, body []byte) error {
	key := c.cacheKey(url, req)

	if err := atomicWrite(c.bodyPath(key), body); err != nil {
		return err
	}

	meta := cachedResponse{
		URL:       url,
		Status:    status,
		Headers:   headers,
		FetchedAt: c.now(),
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
