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

var MocksDir = filepath.Join("testdata", "mocks")

type Recording struct {
	URL        string           `json:"url"`
	Method     string           `json:"method"`
	CapturedAt time.Time        `json:"captured_at"`
	LatencyMs  int64            `json:"latency_ms"`
	Response   RecordedResponse `json:"response"`
}

type RecordedResponse struct {
	StatusCode int         `json:"status_code"`
	Headers    http.Header `json:"headers"`
	BodyBase64 string      `json:"body_base64"`
}

const envRecord = "SEAPORTAL_RECORD_MOCKS"

const envCI = "CI"

type fataler interface {
	Helper()
	Fatalf(format string, args ...any)
}

func Replay(t *testing.T, slug string) http.RoundTripper {
	t.Helper()
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
