package mock

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempMocksDir redirects MocksDir to a per-test tmp dir so the package's
// own tests don't churn the committed testdata/mocks tree.
func withTempMocksDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := MocksDir
	MocksDir = dir
	t.Cleanup(func() { MocksDir = prev })
	return dir
}

func writeFixture(t *testing.T, slug string, body string, headers http.Header, status int) {
	t.Helper()
	rec := Recording{
		URL:       "https://example.test/foo",
		Method:    "GET",
		LatencyMs: 42,
		Response: RecordedResponse{
			StatusCode: status,
			Headers:    headers,
			BodyBase64: base64.StdEncoding.EncodeToString([]byte(body)),
		},
	}
	if err := saveRecording(slug, rec); err != nil {
		t.Fatalf("saveRecording: %v", err)
	}
}

func TestMock_ReplaySuccess(t *testing.T) {
	withTempMocksDir(t)
	const body = "<html><body>hello mock</body></html>"
	writeFixture(t, "replay-ok", body, http.Header{
		"Content-Type": []string{"text/html; charset=utf-8"},
		"X-Foo":        []string{"bar"},
	}, http.StatusOK)

	rt := Replay(t, "replay-ok")
	req, _ := http.NewRequest("GET", "https://anything.test/ignored", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Foo"); got != "bar" {
		t.Errorf("X-Foo = %q, want %q", got, "bar")
	}
	gotBody, _ := io.ReadAll(resp.Body)
	if string(gotBody) != body {
		t.Errorf("body mismatch:\n got: %q\nwant: %q", gotBody, body)
	}
}

func TestMock_ReplayMissingFile(t *testing.T) {
	withTempMocksDir(t)

	// Exercise the missing-file branch through the same helper Replay uses,
	// but with a stub fataler so the parent test isn't aborted by
	// runtime.Goexit. A real *testing.T would call FailNow here.
	stub := &stubFataler{}
	rt := replay(stub, "does-not-exist")
	if rt != nil {
		t.Fatalf("replay returned non-nil RT for missing fixture: %v", rt)
	}
	if !stub.fatalfCalled {
		t.Fatal("expected Fatalf for missing fixture")
	}
	if !strings.Contains(stub.fatalfMsg, "does-not-exist") {
		t.Errorf("Fatalf message missing slug; got %q", stub.fatalfMsg)
	}
	if !strings.Contains(stub.fatalfMsg, envRecord) {
		t.Errorf("Fatalf message missing env-var hint; got %q", stub.fatalfMsg)
	}
}

type stubFataler struct {
	fatalfCalled bool
	fatalfMsg    string
}

func (s *stubFataler) Helper() {}
func (s *stubFataler) Fatalf(format string, args ...any) {
	s.fatalfCalled = true
	s.fatalfMsg = fmt.Sprintf(format, args...)
}

func TestMock_RecordRoundTrip(t *testing.T) {
	dir := withTempMocksDir(t)

	const upstreamBody = "recorded body payload"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Captured", "yes")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, upstreamBody)
	}))
	t.Cleanup(srv.Close)

	t.Setenv(envRecord, "1")
	t.Setenv(envCI, "") // explicit: not in CI

	rt := Record(t, "round-trip")
	req, _ := http.NewRequest("GET", srv.URL+"/path", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip(record): %v", err)
	}
	gotBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(gotBody) != upstreamBody {
		t.Fatalf("recorded RT returned wrong body: %q", gotBody)
	}

	// Verify the recording landed on disk.
	if _, err := os.Stat(filepath.Join(dir, "round-trip.json")); err != nil {
		t.Fatalf("recording file missing: %v", err)
	}

	// Replay must serve the same bytes.
	rt2 := Replay(t, "round-trip")
	replayReq, _ := http.NewRequest("GET", "https://ignored.test/", nil)
	rresp, err := rt2.RoundTrip(replayReq)
	if err != nil {
		t.Fatalf("RoundTrip(replay): %v", err)
	}
	defer func() { _ = rresp.Body.Close() }()
	rbody, _ := io.ReadAll(rresp.Body)
	if string(rbody) != upstreamBody {
		t.Errorf("replay body mismatch: got %q want %q", rbody, upstreamBody)
	}
	if rresp.Header.Get("X-Captured") != "yes" {
		t.Errorf("replay lost X-Captured header: %v", rresp.Header)
	}
	if rresp.StatusCode != http.StatusOK {
		t.Errorf("replay status = %d, want 200", rresp.StatusCode)
	}
	// Sanity: request URL was honoured by record (server got a real call).
	if _, err := url.Parse(srv.URL); err != nil {
		t.Fatalf("server URL bad: %v", err)
	}
}

func TestMock_RecordIgnoredWhenEnvUnset(t *testing.T) {
	dir := withTempMocksDir(t)

	// Pre-seed a fixture so the implicit Replay fallback succeeds.
	const body = "fallback body"
	writeFixture(t, "env-unset", body, http.Header{"Content-Type": []string{"text/plain"}}, http.StatusOK)

	t.Setenv(envRecord, "")

	rt := Record(t, "env-unset")
	if _, ok := rt.(*replayRT); !ok {
		t.Fatalf("Record with unset env should return *replayRT, got %T", rt)
	}

	req, _ := http.NewRequest("GET", "https://anything.test/", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != body {
		t.Errorf("body = %q, want %q (Record should have degraded to Replay)", got, body)
	}

	// And no new file should have been written for an unrelated slug.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected only the pre-seeded fixture on disk, got %d entries", len(entries))
	}
}
