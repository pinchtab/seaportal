// Package mock provides a record/replay HTTP RoundTripper layer for tests
// that exercise the engine's HTTP-fetch paths without paying real-network
// latency or flakiness costs.
//
// Lifecycle
//
//   - Replay (default, hermetic): reads testdata/mocks/<slug>.json and serves
//     the canned response on every RoundTrip. Test fails fast when the slug
//     file is missing — recordings are explicit, deterministic artifacts that
//     should be committed alongside the test.
//
//   - Record (opt-in): activated only when SEAPORTAL_RECORD_MOCKS=1 is set.
//     Wraps the real transport, ferries the response back to the caller, and
//     persists status/headers/body/latency to testdata/mocks/<slug>.json.
//     When the env var is unset, Record degrades to Replay so tests stay
//     hermetic by default.
//
// CI safety: recording is refused when CI=true to prevent accidental
// overwrites from automated pipelines. Recordings should be produced and
// reviewed locally.
//
// On-disk format (testdata/mocks/<slug>.json):
//
//	{
//	  "url": "https://example.com/foo",
//	  "method": "GET",
//	  "captured_at": "2026-05-17T12:34:56Z",
//	  "latency_ms": 142,
//	  "response": {
//	    "status_code": 200,
//	    "headers": {"Content-Type": ["text/html; charset=utf-8"]},
//	    "body_base64": "PGh0bWw+..."
//	  }
//	}
//
// Body is base64-encoded so binary responses (gzip/br/zstd/PDF) round-trip
// cleanly without JSON-escaping pitfalls.
package mock

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// MocksDir is the on-disk directory relative to the test's working directory
// where recordings live. Tests typically run with cwd == package dir, so
// this resolves to <pkg>/testdata/mocks. Exposed so callers (or tests of the
// mock package itself) can override for temp-dir round-trips.
var MocksDir = filepath.Join("testdata", "mocks")

// Recording is the on-disk format. Exported so callers writing custom
// fixtures can construct one directly without going through Record.
type Recording struct {
	URL        string           `json:"url"`
	Method     string           `json:"method"`
	CapturedAt time.Time        `json:"captured_at"`
	LatencyMs  int64            `json:"latency_ms"`
	Response   RecordedResponse `json:"response"`
}

// RecordedResponse mirrors the response fields needed to faithfully replay
// an HTTP response: status, headers, and the raw (still-encoded) body bytes.
type RecordedResponse struct {
	StatusCode int         `json:"status_code"`
	Headers    http.Header `json:"headers"`
	BodyBase64 string      `json:"body_base64"`
}

// envRecord is the env var that switches Record from replay-fallback to
// genuine recording. Documented in the package comment.
const envRecord = "SEAPORTAL_RECORD_MOCKS"

// envCI is the conventional CI flag set by GitHub Actions, GitLab CI, etc.
// Used as a defensive guard against accidental recording in pipelines.
const envCI = "CI"

// fataler is the slice of *testing.T we need. Extracted so tests can drive
// the missing-file path against a stub without spinning a real subtest.
type fataler interface {
	Helper()
	Fatalf(format string, args ...any)
}

// Replay returns an http.RoundTripper that serves the recording stored at
// testdata/mocks/<slug>.json. The incoming request URL is ignored — one slug
// maps to one canned response. Calls t.Fatalf when the slug file is missing
// or malformed; this is the desired behaviour so missing fixtures surface
// loudly rather than silently passing.
func Replay(t *testing.T, slug string) http.RoundTripper {
	return replay(t, slug)
}

func replay(t fataler, slug string) http.RoundTripper {
	t.Helper()
	rec, err := loadRecording(slug)
	if err != nil {
		t.Fatalf("mock.Replay(%q): %v (record with %s=1)", slug, err, envRecord)
		return nil
	}
	return &replayRT{rec: rec}
}

// Record returns a RoundTripper that wraps the live transport and persists
// each response to testdata/mocks/<slug>.json. When SEAPORTAL_RECORD_MOCKS=1
// is unset, Record degrades to Replay so the default test run stays hermetic.
// When CI=true is set alongside the record flag, Record fails fast — CI
// runners should never rewrite fixtures.
//
// The base RoundTripper used for the live request is http.DefaultTransport;
// callers needing the utls Chrome fingerprint should configure that at the
// caller side (or use Record only against test httptest.Server upstreams).
func Record(t *testing.T, slug string) http.RoundTripper {
	t.Helper()
	if os.Getenv(envRecord) != "1" {
		return Replay(t, slug)
	}
	if os.Getenv(envCI) == "true" {
		t.Fatalf("mock.Record(%q): refusing to record while CI=true (would overwrite committed fixtures)", slug)
	}
	if err := os.MkdirAll(MocksDir, 0o755); err != nil {
		t.Fatalf("mock.Record(%q): mkdir %s: %v", slug, MocksDir, err)
	}
	return &recordRT{
		slug: slug,
		base: http.DefaultTransport,
		t:    t,
	}
}

// replayRT serves a single canned response on every RoundTrip.
type replayRT struct {
	rec Recording
}

func (r *replayRT) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := base64.StdEncoding.DecodeString(r.rec.Response.BodyBase64)
	if err != nil {
		return nil, fmt.Errorf("mock replay: decode body: %w", err)
	}
	headers := r.rec.Response.Headers.Clone()
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", r.rec.Response.StatusCode, http.StatusText(r.rec.Response.StatusCode)),
		StatusCode:    r.rec.Response.StatusCode,
		Header:        headers,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
	}, nil
}

// recordRT wraps a real transport, persisting each response to disk.
type recordRT struct {
	slug string
	base http.RoundTripper
	t    *testing.T
}

func (r *recordRT) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := r.base.RoundTrip(req)
	latency := time.Since(start)
	if err != nil {
		return nil, err
	}

	// Read the full body so we can both persist it and hand a fresh reader
	// back to the caller. Fine for fixture-sized payloads; the live engine
	// reads bodies fully anyway.
	bodyBytes, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("mock record: read body: %w", readErr)
	}

	rec := Recording{
		URL:        req.URL.String(),
		Method:     req.Method,
		CapturedAt: time.Now().UTC(),
		LatencyMs:  latency.Milliseconds(),
		Response: RecordedResponse{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header.Clone(),
			BodyBase64: base64.StdEncoding.EncodeToString(bodyBytes),
		},
	}
	if writeErr := saveRecording(r.slug, rec); writeErr != nil {
		r.t.Fatalf("mock.Record(%q): save: %v", r.slug, writeErr)
	}

	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	resp.ContentLength = int64(len(bodyBytes))
	return resp, nil
}

func recordingPath(slug string) string {
	return filepath.Join(MocksDir, slug+".json")
}

func loadRecording(slug string) (Recording, error) {
	var rec Recording
	data, err := os.ReadFile(recordingPath(slug))
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, fmt.Errorf("unmarshal: %w", err)
	}
	return rec, nil
}

func saveRecording(slug string, rec Recording) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(recordingPath(slug), data, 0o644)
}
