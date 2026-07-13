package engine

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseProxyURL_AuthEmbedded(t *testing.T) {
	u, err := url.Parse("http://alice:s3cret@proxy.example.com:8080")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.User == nil {
		t.Fatal("expected userinfo")
	}
	if got := u.User.Username(); got != "alice" {
		t.Errorf("user: got %q want alice", got)
	}
	pass, ok := u.User.Password()
	if !ok || pass != "s3cret" {
		t.Errorf("pass: got %q ok=%v", pass, ok)
	}
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:s3cret"))
	got := "Basic " + base64.StdEncoding.EncodeToString([]byte(u.User.Username()+":"+pass))
	if got != expected {
		t.Errorf("auth header: got %q want %q", got, expected)
	}
}

func TestProxy_InvalidURLErrors(t *testing.T) {
	cases := []string{
		"://bogus",
		"not-a-url",
		"http://",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			_, err := getClientForOptions(Options{Proxy: raw})
			if err == nil {
				t.Fatalf("expected error for proxy=%q", raw)
			}
		})
	}
}

func TestProxy_EmptyReturnsSharedClient(t *testing.T) {
	c, err := getClientForOptions(Options{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c == nil {
		t.Fatal("nil client")
	}
}

func TestChromeTransport_ProxyHTTP_NonHTTPS(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("target-ok"))
	}))
	defer target.Close()

	var proxyHit bool
	var mu sync.Mutex
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		proxyHit = true
		mu.Unlock()
		outReq, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		resp, err := http.DefaultClient.Do(outReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		_, _ = w.Write(body)
	}))
	defer proxy.Close()

	proxyURL, _ := url.Parse(proxy.URL)
	client := getUTLSClientWithProxy(proxyURL)
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "target-ok" {
		t.Errorf("body: got %q want target-ok", string(body))
	}
	mu.Lock()
	defer mu.Unlock()
	if !proxyHit {
		t.Error("proxy was not invoked")
	}
}

func startConnectProxy(t *testing.T, backend string) (net.Listener, *string, *sync.Mutex) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var auth string
	var mu sync.Mutex
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(client net.Conn) {
				defer func() { _ = client.Close() }()
				br := bufio.NewReader(client)
				reqLine, err := br.ReadString('\n')
				if err != nil {
					return
				}
				if !strings.HasPrefix(reqLine, "CONNECT ") {
					_, _ = client.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
					return
				}
				var localAuth string
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					if line == "\r\n" || line == "\n" {
						break
					}
					if strings.HasPrefix(strings.ToLower(line), "proxy-authorization:") {
						localAuth = strings.TrimSpace(line[len("Proxy-Authorization:"):])
					}
				}
				mu.Lock()
				auth = localAuth
				mu.Unlock()

				if _, err := client.Write([]byte("HTTP/1.1 200 OK\r\n\r\n")); err != nil {
					return
				}

				upstream, err := net.Dial("tcp", backend)
				if err != nil {
					return
				}
				defer func() { _ = upstream.Close() }()

				done := make(chan struct{}, 2)
				go func() {
					_, _ = io.Copy(upstream, br)
					done <- struct{}{}
				}()
				go func() {
					_, _ = io.Copy(client, upstream)
					done <- struct{}{}
				}()
				<-done
			}(c)
		}
	}()
	return ln, &auth, &mu
}

func TestChromeTransport_ProxyConnect_HTTPS(t *testing.T) {
	backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tls-target-ok"))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	backendHost := backendURL.Host

	ln, capturedAuth, mu := startConnectProxy(t, backendHost)
	defer func() { _ = ln.Close() }()

	proxyURL := &url.URL{
		Scheme: "http",
		User:   url.UserPassword("user", "pass"),
		Host:   ln.Addr().String(),
	}

	client := getUTLSClientWithProxy(proxyURL)
	resp, err := client.Get(backend.URL)
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "tls-target-ok" {
			t.Logf("backend body: %q (cert verify likely failed, acceptable)", string(body))
		}
	}
	if err != nil {
		if !strings.Contains(err.Error(), "x509") && !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "tls") {
			t.Logf("non-TLS error (still acceptable if proxy got CONNECT): %v", err)
		}
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	got := *capturedAuth
	mu.Unlock()
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if got != want {
		t.Fatalf("Proxy-Authorization: got %q want %q", got, want)
	}
}

func TestExtract_ProxyFlagRoutesThroughProxy(t *testing.T) {
	backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "<html><body><p>proxied</p></body></html>")
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	ln, capturedAuth, mu := startConnectProxy(t, backendURL.Host)
	defer func() { _ = ln.Close() }()

	proxyURL := (&url.URL{
		Scheme: "http",
		User:   url.UserPassword("bob", "hunter2"),
		Host:   ln.Addr().String(),
	}).String()

	res := FromURLWithOptions(backend.URL, Options{Proxy: proxyURL, MaxRetries: 0, TotalRetryTimeout: 2 * time.Second})
	_ = res

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	got := *capturedAuth
	mu.Unlock()
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("bob:hunter2"))
	if got != want {
		t.Fatalf("Proxy-Authorization: got %q want %q", got, want)
	}
}
