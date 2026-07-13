package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
)

func allowInternalPolicy() *SecurityPolicy {
	return &SecurityPolicy{BlockPrivateIPs: false}
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func brotliBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	bw := brotli.NewWriter(&buf)
	if _, err := bw.Write(data); err != nil {
		t.Fatalf("brotli write: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("brotli close: %v", err)
	}
	return buf.Bytes()
}

func TestFetchBytes_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "text/html" {
			t.Errorf("Accept: got %q want %q", got, "text/html")
		}
		if got := r.Header.Get("User-Agent"); got != DefaultUserAgent {
			t.Errorf("User-Agent: got %q want default", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Custom", "hello")
		_, _ = w.Write([]byte("<html>ok</html>"))
	}))
	defer srv.Close()

	body, headers, status, err := FetchBytes(context.Background(), srv.URL, FetchBytesOptions{
		Accept: "text/html",
	})
	if err != nil {
		t.Fatalf("FetchBytes: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status: got %d want 200", status)
	}
	if string(body) != "<html>ok</html>" {
		t.Errorf("body: got %q", body)
	}
	if headers.Get("X-Custom") != "hello" {
		t.Errorf("headers: lost X-Custom, got %v", headers)
	}
}

func TestFetchBytes_CustomUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "seaportal-test/1.0" {
			t.Errorf("User-Agent: got %q", got)
		}
	}))
	defer srv.Close()

	if _, _, _, err := FetchBytes(context.Background(), srv.URL, FetchBytesOptions{
		UserAgent: "  seaportal-test/1.0  ",
	}); err != nil {
		t.Fatalf("FetchBytes: %v", err)
	}
}

func TestFetchBytes_Non2xxPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("gone fishing"))
	}))
	defer srv.Close()

	body, _, status, err := FetchBytes(context.Background(), srv.URL, FetchBytesOptions{})
	if err != nil {
		t.Fatalf("FetchBytes: %v", err)
	}
	if status != http.StatusNotFound {
		t.Errorf("status: got %d want 404", status)
	}
	if string(body) != "gone fishing" {
		t.Errorf("body: got %q", body)
	}
}

func TestFetchBytes_SecurityBlocksInternalTarget(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	defer srv.Close()

	_, _, status, err := FetchBytes(context.Background(), srv.URL, FetchBytesOptions{
		Security: DefaultSecurityPolicy(),
	})
	if !errors.Is(err, ErrPrivateIPBlocked) {
		t.Fatalf("err: got %v, want ErrPrivateIPBlocked", err)
	}
	if status != 0 {
		t.Errorf("status: got %d want 0 (blocked pre-fetch)", status)
	}
	if hits != 0 {
		t.Errorf("server hits: got %d want 0", hits)
	}
}

func TestFetchBytes_AllowInternalPolicyPermitsLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("internal ok"))
	}))
	defer srv.Close()

	body, _, status, err := FetchBytes(context.Background(), srv.URL, FetchBytesOptions{
		Security: allowInternalPolicy(),
	})
	if err != nil {
		t.Fatalf("FetchBytes: %v", err)
	}
	if status != http.StatusOK || string(body) != "internal ok" {
		t.Errorf("got status=%d body=%q", status, body)
	}
}

func TestFetchBytes_SecuritySchemeBlocked(t *testing.T) {
	sec := allowInternalPolicy()
	sec.AllowedSchemes = []string{"https"}
	_, _, _, err := FetchBytes(context.Background(), "http://127.0.0.1:1/x", FetchBytesOptions{Security: sec})
	if !errors.Is(err, ErrSecurityScheme) {
		t.Fatalf("err: got %v, want ErrSecurityScheme", err)
	}
}

func TestFetchBytes_ResponseSizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 1024))
	}))
	defer srv.Close()

	sec := allowInternalPolicy()
	sec.MaxResponseBytes = 64
	_, _, status, err := FetchBytes(context.Background(), srv.URL, FetchBytesOptions{Security: sec})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err: got %v, want ErrResponseTooLarge", err)
	}
	if status != http.StatusOK {
		t.Errorf("status: got %d want 200 (cap hit while reading body)", status)
	}
}

func TestFetchBytes_GzipDecompression(t *testing.T) {
	const plain = "hello gzip world, this is the decompressed payload"
	payload := gzipBytes(t, []byte(plain))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	body, _, status, err := FetchBytes(context.Background(), srv.URL, FetchBytesOptions{Client: client})
	if err != nil {
		t.Fatalf("FetchBytes: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status: got %d", status)
	}
	if string(body) != plain {
		t.Errorf("body: got %q want decompressed %q", body, plain)
	}
}

func TestFetchBytes_BrotliDecompression(t *testing.T) {
	const plain = "hello brotli world, this is the decompressed payload"
	payload := brotliBytes(t, []byte(plain))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "br")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	body, _, _, err := FetchBytes(context.Background(), srv.URL, FetchBytesOptions{})
	if err != nil {
		t.Fatalf("FetchBytes: %v", err)
	}
	if string(body) != plain {
		t.Errorf("body: got %q want decompressed %q", body, plain)
	}
}

func TestFetchBytes_DecompressionBombCap(t *testing.T) {
	payload := gzipBytes(t, bytes.Repeat([]byte("0"), 64*1024))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	sec := allowInternalPolicy()
	sec.MaxDecompressedBytes = 512
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	_, _, status, err := FetchBytes(context.Background(), srv.URL, FetchBytesOptions{Client: client, Security: sec})
	if !errors.Is(err, ErrDecompressTooLarge) {
		t.Fatalf("err: got %v, want ErrDecompressTooLarge", err)
	}
	if status != http.StatusOK {
		t.Errorf("status: got %d want 200 (cap hit while decompressing)", status)
	}
}

func TestFetchBytes_RedirectsFollowedUnderCap(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/b", http.StatusFound)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("final stop"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	sec := allowInternalPolicy()
	sec.MaxRedirects = 5
	body, _, status, err := FetchBytes(context.Background(), srv.URL+"/a", FetchBytesOptions{Security: sec})
	if err != nil {
		t.Fatalf("FetchBytes: %v", err)
	}
	if status != http.StatusOK || string(body) != "final stop" {
		t.Errorf("got status=%d body=%q", status, body)
	}
}

func TestFetchBytes_RedirectCapReturnsLastResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/b", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirect target must not be fetched with MaxRedirects=0")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	sec := allowInternalPolicy()
	sec.MaxRedirects = 0
	_, headers, status, err := FetchBytes(context.Background(), srv.URL+"/a", FetchBytesOptions{Security: sec})
	if err != nil {
		t.Fatalf("FetchBytes: %v", err)
	}
	if status != http.StatusMovedPermanently {
		t.Errorf("status: got %d want 301", status)
	}
	if loc := headers.Get("Location"); !strings.HasSuffix(loc, "/b") {
		t.Errorf("Location: got %q", loc)
	}
}

func TestFetchBytes_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must not be reached with a pre-cancelled context")
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err := FetchBytes(ctx, srv.URL, FetchBytesOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err: got %v, want context.Canceled", err)
	}
}

func TestFetchBytes_InvalidURL(t *testing.T) {
	_, _, _, err := FetchBytes(context.Background(), "http://[::1]:namedport/x", FetchBytesOptions{})
	if err == nil {
		t.Fatalf("expected request-construction error for malformed URL")
	}
}

func TestFetchBytes_TimeoutOverride(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	start := time.Now()
	_, _, _, err := FetchBytes(context.Background(), srv.URL, FetchBytesOptions{
		Timeout: 50 * time.Millisecond,
	})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("timeout not applied: took %s", elapsed)
	}
}
