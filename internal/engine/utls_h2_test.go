package engine

import (
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	utls "github.com/refraction-networking/utls"
)

// withTestTLSTrust installs a process-wide test hook that trusts srv's
// self-signed certificate inside dialTLSChrome. The hook is package-internal,
// nil in production. Returns a cleanup func.
func withTestTLSTrust(t *testing.T, srv *httptest.Server) func() {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	prev := testTLSConfig
	testTLSConfig = &utls.Config{RootCAs: pool}
	return func() { testTLSConfig = prev }
}

// startTLSServer spins up an httptest TLS server with the given ALPN protocol
// list. When "h2" is offered the server is started with EnableHTTP2 so Go's
// h2 server actually handles the connection.
func startTLSServer(t *testing.T, nextProtos []string, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.EnableHTTP2 = false
	for _, p := range nextProtos {
		if p == "h2" {
			srv.EnableHTTP2 = true
			break
		}
	}
	srv.StartTLS()
	// Override NextProtos after StartTLS so we can force "http/1.1"-only.
	if srv.TLS != nil {
		srv.TLS.NextProtos = nextProtos
	}
	t.Cleanup(srv.Close)
	return srv
}

func TestChromeTransport_NegotiatesH2WhenAvailable(t *testing.T) {
	srv := startTLSServer(t, []string{"h2", "http/1.1"}, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "hello h2")
	})
	cleanup := withTestTLSTrust(t, srv)
	defer cleanup()

	client := &http.Client{Transport: &chromeTransport{}}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hello h2") {
		t.Errorf("body mismatch: %q", string(body))
	}
	if resp.TLS == nil {
		t.Fatal("expected resp.TLS to be populated")
	}
	if got := resp.TLS.NegotiatedProtocol; got != "h2" {
		t.Errorf("NegotiatedProtocol: got %q, want h2", got)
	}
}

func TestChromeTransport_FallsBackToHTTP1(t *testing.T) {
	srv := startTLSServer(t, []string{"http/1.1"}, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "hello h1")
	})
	cleanup := withTestTLSTrust(t, srv)
	defer cleanup()

	client := &http.Client{Transport: &chromeTransport{}}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)
	if resp.TLS == nil {
		t.Fatal("expected resp.TLS to be populated")
	}
	if got := resp.TLS.NegotiatedProtocol; got != "http/1.1" {
		t.Errorf("NegotiatedProtocol: got %q, want http/1.1", got)
	}
}

func TestExtract_ProtocolFieldPopulatedOnHTTPS(t *testing.T) {
	srv := startTLSServer(t, []string{"h2", "http/1.1"}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintln(w, "<html><head><title>P</title></head><body><p>hi</p></body></html>")
	})
	cleanup := withTestTLSTrust(t, srv)
	defer cleanup()

	result := FromURLWithOptions(srv.URL, Options{})
	if result.Error != "" {
		t.Fatalf("extraction error: %s", result.Error)
	}
	if result.Protocol != "h2" {
		t.Errorf("Result.Protocol: got %q, want h2", result.Protocol)
	}
}
