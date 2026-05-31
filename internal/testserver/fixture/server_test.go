package fixture_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/seaportal/internal/testserver/fixture"
)

func TestFixture_StatusCode(t *testing.T) {
	srv := fixture.New().Route("GET", "/status/406", fixture.Status(406))
	defer srv.Close()

	resp, err := http.Get(srv.URL() + "/status/406")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 406 {
		t.Fatalf("status: got %d, want 406", resp.StatusCode)
	}
}

func TestFixture_Redirect(t *testing.T) {
	srv := fixture.New().Route("GET", "/redirect", fixture.Redirect("/final", 301))
	defer srv.Close()

	// Use a client that doesn't follow redirects so we can observe the 301.
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(srv.URL() + "/redirect")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 301 {
		t.Fatalf("status: got %d, want 301", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/final" {
		t.Fatalf("Location: got %q, want %q", loc, "/final")
	}
}

func TestFixture_Delay(t *testing.T) {
	const delay = 150 * time.Millisecond
	srv := fixture.New().Route("GET", "/slow", fixture.Delay(delay, fixture.Status(200)))
	defer srv.Close()

	start := time.Now()
	resp, err := http.Get(srv.URL() + "/slow")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if elapsed < delay {
		t.Fatalf("elapsed %v < delay %v — Delay did not sleep", elapsed, delay)
	}
}

func TestFixture_Charset(t *testing.T) {
	// 0xE9 is "é" in iso-8859-1, not valid UTF-8.
	body := []byte{'h', 'e', 'l', 'l', 'o', ' ', 0xE9}
	srv := fixture.New().Route("GET", "/latin1", fixture.Charset("iso-8859-1", body))
	defer srv.Close()

	resp, err := http.Get(srv.URL() + "/latin1")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	got := resp.Header.Get("Content-Type")
	want := "text/html; charset=iso-8859-1"
	if got != want {
		t.Fatalf("Content-Type: got %q, want %q", got, want)
	}
	gotBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(gotBody) != string(body) {
		t.Fatalf("body: got %v, want %v", gotBody, body)
	}
}

func TestFixture_Body(t *testing.T) {
	srv := fixture.New().Route("GET", "/final", fixture.Body([]byte("ok"), "text/plain"))
	defer srv.Close()

	resp, err := http.Get(srv.URL() + "/final")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("Content-Type: got %q, want %q", ct, "text/plain")
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(b) != "ok" {
		t.Fatalf("body: got %q, want %q", string(b), "ok")
	}
}

func TestFixture_MethodMismatch(t *testing.T) {
	srv := fixture.New().Route("GET", "/only-get", fixture.Status(200))
	defer srv.Close()

	resp, err := http.Post(srv.URL()+"/only-get", "text/plain", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}
