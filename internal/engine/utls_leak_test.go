package engine

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pinchtab/seaportal/internal/engine/leakcheck"
)

func startCountingH2Server(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var conns atomic.Int32
	srv := httptest.NewUnstartedServer(handler)
	srv.EnableHTTP2 = true
	srv.Config.ConnState = func(c net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, &conns
}

func TestChromeTransport_H2NoGoroutineLeak(t *testing.T) {
	leakcheck.CheckLeak(t)

	srv, conns := startCountingH2Server(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "leak-free")
	})
	const n = 20
	client := &http.Client{Transport: &chromeTransport{tlsConfig: testTLSTrust(t, srv)}, Timeout: 10 * time.Second}
	for i := 0; i < n; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			t.Fatalf("request %d read body: %v", i, err)
		}
		_ = resp.Body.Close()
		if got := resp.TLS.NegotiatedProtocol; got != "h2" {
			t.Fatalf("request %d: NegotiatedProtocol = %q, want h2", i, got)
		}
	}

	if got := conns.Load(); got != 1 {
		t.Errorf("server accepted %d conns for %d requests, want 1 (connection reuse)", got, n)
	}

	client.CloseIdleConnections()
}

func TestChromeTransport_H2NoGoroutineLeak_Concurrent(t *testing.T) {
	leakcheck.CheckLeak(t)

	srv, _ := startCountingH2Server(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "concurrent-ok")
	})
	client := &http.Client{Transport: &chromeTransport{tlsConfig: testTLSTrust(t, srv)}, Timeout: 10 * time.Second}
	const workers = 8
	const perWorker = 5
	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				resp, err := client.Get(srv.URL)
				if err != nil {
					errs <- err
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent request: %v", err)
	}

	client.CloseIdleConnections()
}

func TestChromeTransport_H2EvictsDeadConnAndRedials(t *testing.T) {
	leakcheck.CheckLeak(t)

	srv, conns := startCountingH2Server(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "redial-ok")
	})
	tr := &chromeTransport{tlsConfig: testTLSTrust(t, srv)}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	get := func(label string) {
		t.Helper()
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	get("first request")
	if got := conns.Load(); got != 1 {
		t.Fatalf("after first request: %d conns, want 1", got)
	}

	srv.CloseClientConnections()
	key := canonicalHostPort(mustParseURL(t, srv.URL).Host)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tr.mu.Lock()
		cc := tr.h2Conns[key]
		tr.mu.Unlock()
		if cc == nil || !cc.CanTakeNewRequest() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	get("request after server dropped conn")
	if got := conns.Load(); got != 2 {
		t.Errorf("after redial: %d conns, want 2 (one eviction + one fresh dial)", got)
	}

	client.CloseIdleConnections()
}

func TestChromeTransport_CloseIdleConnections_EmptiesCache(t *testing.T) {
	leakcheck.CheckLeak(t)

	srv, conns := startCountingH2Server(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "ok")
	})
	tr := &chromeTransport{tlsConfig: testTLSTrust(t, srv)}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	for i := 0; i < 2; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		client.CloseIdleConnections()

		tr.mu.Lock()
		cached := len(tr.h2Conns)
		tr.mu.Unlock()
		if cached != 0 {
			t.Fatalf("round %d: %d conns still cached after CloseIdleConnections", i, cached)
		}
	}
	if got := conns.Load(); got != 2 {
		t.Errorf("server saw %d conns, want 2 (cache emptied between requests)", got)
	}
}

func TestChromeTransport_PlainTransportBuiltOnce(t *testing.T) {
	proxyURL, _ := url.Parse("http://proxy.example.com:8080")
	cases := []struct {
		name string
		tr   *chromeTransport
		want string
	}{
		{"proxy", &chromeTransport{proxyURL: proxyURL}, "owned"},
		{"security dial guard", &chromeTransport{security: &SecurityPolicy{BlockPrivateIPs: true}}, "owned"},
		{"bare", &chromeTransport{}, "default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first := tc.tr.plainTransport()
			second := tc.tr.plainTransport()
			if first != second {
				t.Errorf("plainTransport not reused: %p vs %p", first, second)
			}
			isDefault := first == http.DefaultTransport
			if tc.want == "default" && !isDefault {
				t.Errorf("bare transport should fall through to http.DefaultTransport")
			}
			if tc.want == "owned" && isDefault {
				t.Errorf("%s branch should own its transport, got http.DefaultTransport", tc.name)
			}
			if tc.want == "owned" {
				if _, ok := first.(*http.Transport); !ok {
					t.Errorf("owned transport should be *http.Transport, got %T", first)
				}
			}
		})
	}
}

func TestChromeTransport_H2TransportSharedAcrossDials(t *testing.T) {
	tr := &chromeTransport{}
	first := tr.h2Transport()
	second := tr.h2Transport()
	if first != second {
		t.Errorf("h2Transport not reused: %p vs %p", first, second)
	}
	if first.IdleConnTimeout != h2IdleConnTimeout {
		t.Errorf("IdleConnTimeout = %v, want %v", first.IdleConnTimeout, h2IdleConnTimeout)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}
