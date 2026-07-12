package engine

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// testTLSConfig, when non-nil, is merged into the utls.Config used by
// dialTLSChrome before handshake. Test-only hook so httptest TLS servers
// (self-signed certs) can be reached without weakening the production path.
// Production callers MUST leave this nil.
var testTLSConfig *utls.Config

var (
	utlsClient     *http.Client
	utlsClientOnce sync.Once
)

// h2IdleConnTimeout is how long a cached idle HTTP/2 connection may sit
// unused before closing itself (mirrors net/http.DefaultTransport's
// IdleConnTimeout). The self-close also bounds the lifetime of connections
// cached by short-lived chromeTransport instances that nobody explicitly
// closes.
const h2IdleConnTimeout = 90 * time.Second

// h2DrainTimeout bounds the graceful drain of an evicted HTTP/2 connection
// before it is force-closed, so a wedged peer can't pin the connection's
// readLoop goroutine (and fd) forever.
const h2DrainTimeout = 5 * time.Second

// getUTLSClient returns a shared HTTP client using utls for Chrome fingerprint impersonation.
// This bypasses Cloudflare and other bot detection that fingerprint TLS.
func getUTLSClient() *http.Client {
	utlsClientOnce.Do(func() {
		utlsClient = &http.Client{
			Timeout:   30 * time.Second,
			Transport: &chromeTransport{},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}
	})
	return utlsClient
}

// getUTLSClientWithProxy returns a fresh (uncached) HTTP client that routes
// through the supplied proxy URL. HTTPS targets are tunnelled via CONNECT
// with optional Basic auth (from proxyURL.User); the Chrome TLS fingerprint
// is applied AFTER the tunnel is established. HTTP targets are forwarded via
// a vanilla http.Transport with Proxy set, which natively handles Basic auth
// and SOCKS5.
func getUTLSClientWithProxy(proxyURL *url.URL) *http.Client {
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &chromeTransport{proxyURL: proxyURL},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

// chromeTransport implements http.RoundTripper with Chrome TLS fingerprint.
// Handles both HTTP/1.1 and HTTP/2 depending on server ALPN negotiation.
// Optional proxyURL routes requests through an HTTP/HTTPS/SOCKS5 proxy.
//
// security, when set with BlockPrivateIPs, installs a dial Control hook that
// re-validates the post-DNS resolved IP just before connect — the
// DNS-rebinding guard. It is applied only on the direct (non-proxy) dial: a
// proxied request dials the proxy, not the target, so the target's host/scheme
// is instead vetted by the pre-fetch and per-redirect ValidateURL gates.
//
// HTTP/2 connections are cached per host (h2Conns) and reused across
// requests, so a long-running process performs one TLS handshake per host
// instead of leaking a connection + readLoop goroutine per request. The
// transport is shared across scrape-pool goroutines; mu guards all lazily
// built state. chromeTransport is constructed as a bare struct literal at
// several call sites, so everything derived must be built lazily — there is
// deliberately no constructor.
type chromeTransport struct {
	proxyURL *url.URL
	security *SecurityPolicy

	mu      sync.Mutex
	h2Trans *http2.Transport             // shared wrapper for dialled h2 conns
	h2Conns map[string]*http2.ClientConn // keyed by canonical host:port
	plainTr http.RoundTripper            // non-HTTPS path (T09: built once)
}

func (t *chromeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return t.plainTransport().RoundTrip(req)
	}

	key := canonicalHostPort(req.URL.Host)

	// Fast path: reuse the cached HTTP/2 connection for this host.
	if cc := t.cachedH2Conn(key); cc != nil {
		resp, err := cc.RoundTrip(req)
		if err == nil {
			return withNegotiatedALPN(resp, "h2"), nil
		}
		if cc.CanTakeNewRequest() || req.Context().Err() != nil {
			// Stream-level failure on a still-healthy conn, or the caller's
			// context died: keep the conn, surface the error.
			return nil, err
		}
		// Conn-level failure (GOAWAY, idle self-close, dead peer): evict the
		// stale conn and fall through to a single fresh dial rather than
		// returning a hard error.
		t.evictH2Conn(key, cc)
		var ok bool
		if req, ok = rewindRequest(req); !ok {
			return nil, err
		}
	}

	// HTTPS: dial directly or through a CONNECT tunnel, then upgrade to
	// the utls Chrome fingerprint.
	tlsConn, err := t.dialTLS(req)
	if err != nil {
		return nil, err
	}

	alpn := tlsConn.ConnectionState().NegotiatedProtocol

	var resp *http.Response
	if alpn == "h2" {
		resp, err = t.doHTTP2Request(key, tlsConn, req)
	} else {
		resp, err = doHTTP1Request(tlsConn, req)
		if err != nil {
			_ = tlsConn.Close()
		}
	}
	if err != nil {
		return nil, err
	}

	return withNegotiatedALPN(resp, alpn), nil
}

// dialTLS establishes the fingerprinted TLS connection for req, directly or
// through the configured CONNECT proxy.
func (t *chromeTransport) dialTLS(req *http.Request) (*utls.UConn, error) {
	if t.proxyURL != nil {
		return dialTLSChromeViaProxy(req.Context(), t.proxyURL, req.URL.Hostname(), req.URL.Host)
	}
	return dialTLSChrome(req.Context(), req.URL.Hostname(), req.URL.Host, t.security.dialControl())
}

// plainTransport returns the RoundTripper for non-HTTPS requests. Built once
// and reused — proxyURL and security are immutable after construction — so
// the proxy / dial-guard branches no longer abandon a fresh idle pool on
// every call (T09).
func (t *chromeTransport) plainTransport() http.RoundTripper {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.plainTr == nil {
		// IdleConnTimeout so pools of transports abandoned by short-lived
		// clients self-reap instead of pinning fds (a bare http.Transport
		// keeps idle conns forever).
		switch {
		case t.proxyURL != nil:
			t.plainTr = &http.Transport{
				Proxy:           http.ProxyURL(t.proxyURL),
				IdleConnTimeout: 90 * time.Second,
			}
		default:
			if ctrl := t.security.dialControl(); ctrl != nil {
				// Plain-HTTP direct path with the SSRF dial guard. Note the
				// guard is a dial-time check: pooled conns were validated at
				// connect and a reused TCP conn can't be re-pointed by a DNS
				// rebind, so reuse is safe.
				t.plainTr = &http.Transport{
					DialContext: (&net.Dialer{
						Timeout:   30 * time.Second,
						KeepAlive: 30 * time.Second,
						Control:   ctrl,
					}).DialContext,
					IdleConnTimeout: 90 * time.Second,
				}
			} else {
				t.plainTr = http.DefaultTransport
			}
		}
	}
	return t.plainTr
}

// h2Transport lazily builds the shared http2.Transport used to wrap dialled
// TLS connections. IdleConnTimeout makes each cached conn close itself once
// idle, so even transports abandoned without CloseIdleConnections can't pin
// fds indefinitely.
func (t *chromeTransport) h2Transport() *http2.Transport {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.h2Trans == nil {
		t.h2Trans = &http2.Transport{IdleConnTimeout: h2IdleConnTimeout}
	}
	return t.h2Trans
}

// cachedH2Conn returns the cached connection for key when it can still take
// a new request. A conn that can't (closed, GOAWAY'd, idle-timed-out) is
// evicted and drained so the caller dials fresh.
func (t *chromeTransport) cachedH2Conn(key string) *http2.ClientConn {
	t.mu.Lock()
	cc, ok := t.h2Conns[key]
	if ok && !cc.CanTakeNewRequest() {
		delete(t.h2Conns, key)
		t.mu.Unlock()
		go drainH2Conn(cc)
		return nil
	}
	t.mu.Unlock()
	if !ok {
		return nil
	}
	return cc
}

// evictH2Conn removes cc from the cache — only if it is still the cached
// entry for key; a racing goroutine may have already replaced it — and
// drains it in the background.
func (t *chromeTransport) evictH2Conn(key string, cc *http2.ClientConn) {
	t.mu.Lock()
	if t.h2Conns[key] == cc {
		delete(t.h2Conns, key)
	}
	t.mu.Unlock()
	go drainH2Conn(cc)
}

// cacheH2Conn wraps tlsConn in an http2.ClientConn and installs it in the
// per-host cache. If another goroutine cached a usable conn for the same host
// while this one was dialling, the fresh conn is closed (it has no streams
// yet) and the winner is returned, so a burst of first requests converges on
// one connection. Ownership of tlsConn transfers to the returned ClientConn.
func (t *chromeTransport) cacheH2Conn(key string, tlsConn *utls.UConn) (*http2.ClientConn, error) {
	cc, err := t.h2Transport().NewClientConn(tlsConn)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	if existing, ok := t.h2Conns[key]; ok {
		if existing.CanTakeNewRequest() {
			t.mu.Unlock()
			_ = cc.Close()
			return existing, nil
		}
		// Stale entry: replace it and drain the old conn (it may still be
		// serving another goroutine's in-flight streams).
		go drainH2Conn(existing)
	}
	if t.h2Conns == nil {
		t.h2Conns = make(map[string]*http2.ClientConn)
	}
	t.h2Conns[key] = cc
	t.mu.Unlock()
	return cc, nil
}

// CloseIdleConnections releases every cached HTTP/2 connection and the plain
// transport's idle pool. http.Client.CloseIdleConnections forwards here.
// Idle conns close immediately; conns with in-flight streams are drained
// gracefully rather than interrupted.
func (t *chromeTransport) CloseIdleConnections() {
	t.mu.Lock()
	conns := t.h2Conns
	t.h2Conns = nil
	plain := t.plainTr
	t.mu.Unlock()

	for _, cc := range conns {
		if cc.State().StreamsActive == 0 {
			_ = cc.Close()
		} else {
			go drainH2Conn(cc)
		}
	}
	if tr, ok := plain.(*http.Transport); ok && tr != http.DefaultTransport {
		tr.CloseIdleConnections()
	}
}

// drainH2Conn gracefully shuts down an evicted connection: send GOAWAY, wait
// for in-flight streams (other goroutines may still be reading responses),
// then close. Falls back to a hard Close when the drain outlives
// h2DrainTimeout so a wedged peer can't pin the readLoop goroutine forever.
func drainH2Conn(cc *http2.ClientConn) {
	ctx, cancel := context.WithTimeout(context.Background(), h2DrainTimeout)
	defer cancel()
	if err := cc.Shutdown(ctx); err != nil {
		_ = cc.Close()
	}
}

// rewindRequest prepares req for a one-shot replay after a conn-level failure
// on a cached connection. Bodyless requests replay as-is; requests with a
// GetBody rewind through it; anything else is not replayable because the
// first attempt may have consumed the body.
func rewindRequest(req *http.Request) (*http.Request, bool) {
	if req.Body == nil || req.Body == http.NoBody {
		return req, true
	}
	if req.GetBody == nil {
		return nil, false
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, false
	}
	req2 := req.Clone(req.Context())
	req2.Body = body
	return req2, true
}

// withNegotiatedALPN surfaces the negotiated ALPN protocol on resp.TLS so
// callers can read `resp.TLS.NegotiatedProtocol` exactly as they would with
// the standard transport. Only synthesises when the response doesn't already
// carry one (the h2 layer may attach its own ConnectionState).
func withNegotiatedALPN(resp *http.Response, alpn string) *http.Response {
	if resp.TLS == nil {
		resp.TLS = &tls.ConnectionState{NegotiatedProtocol: alpn}
	}
	return resp
}

// canonicalHostPort normalises a URL host to host:port form (default 443) so
// cache keys for "example.com" and "example.com:443" collide as intended.
func canonicalHostPort(host string) string {
	if _, _, err := net.SplitHostPort(host); err != nil {
		return net.JoinHostPort(host, "443")
	}
	return host
}

// dialTLSChrome establishes a TLS connection impersonating Chrome 120. The
// optional control hook (from SecurityPolicy.dialControl) validates the
// post-DNS resolved IP before connect, closing the DNS-rebinding window.
func dialTLSChrome(ctx context.Context, serverName, host string, control func(network, address string, c syscall.RawConn) error) (*utls.UConn, error) {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   control,
	}

	host = canonicalHostPort(host)

	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}

	cfg := &utls.Config{
		ServerName: serverName,
	}
	if testTLSConfig != nil {
		if testTLSConfig.InsecureSkipVerify {
			cfg.InsecureSkipVerify = true
		}
		if testTLSConfig.RootCAs != nil {
			cfg.RootCAs = testTLSConfig.RootCAs
		}
	}
	tlsConn := utls.UClient(conn, cfg, utls.HelloChrome_120)

	if err := tlsConn.Handshake(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return tlsConn, nil
}

// dialTLSChromeViaProxy opens a CONNECT tunnel to the proxy, then upgrades
// the tunnelled connection to utls Chrome 120 TLS. Fingerprint is applied
// AFTER the tunnel is established, preserving the Chrome TLS signature
// end-to-end with the origin server.
func dialTLSChromeViaProxy(ctx context.Context, proxyURL *url.URL, serverName, host string) (*utls.UConn, error) {
	host = canonicalHostPort(host)

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	proxyAddr := proxyURL.Host
	if proxyAddr == "" {
		return nil, fmt.Errorf("proxy URL missing host")
	}

	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("dial proxy %s: %w", proxyAddr, err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "CONNECT %s HTTP/1.1\r\n", host)
	fmt.Fprintf(&sb, "Host: %s\r\n", host)
	if proxyURL.User != nil {
		user := proxyURL.User.Username()
		pass, _ := proxyURL.User.Password()
		creds := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		fmt.Fprintf(&sb, "Proxy-Authorization: Basic %s\r\n", creds)
	}
	sb.WriteString("\r\n")

	if _, err := conn.Write([]byte(sb.String())); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("write CONNECT: %w", err)
	}

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	statusLine = strings.TrimRight(statusLine, "\r\n")
	if !strings.Contains(statusLine, " 200") {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy CONNECT failed: %s", statusLine)
	}
	// Drain remaining response headers until empty line.
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("read CONNECT headers: %w", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	// If the proxy buffered extra bytes beyond the CONNECT response, we'd
	// lose them by wrapping `conn` directly. In practice servers don't
	// pipeline data before the client's first TLS ClientHello, so this is
	// safe for the CONNECT case.
	if br.Buffered() > 0 {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy sent unexpected pre-handshake bytes")
	}

	tlsConn := utls.UClient(conn, &utls.Config{
		ServerName: serverName,
	}, utls.HelloChrome_120)

	if err := tlsConn.Handshake(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("tls handshake via proxy: %w", err)
	}

	return tlsConn, nil
}

// doHTTP2Request performs an HTTP/2 request over the given TLS connection via
// the per-host connection cache. On success the conn stays cached for reuse
// (resp.Body.Close only ends the stream, which is now correct — the conn is
// pooled, not leaked); on failure it is evicted so the next request redials.
func (t *chromeTransport) doHTTP2Request(key string, tlsConn *utls.UConn, req *http.Request) (*http.Response, error) {
	cc, err := t.cacheH2Conn(key, tlsConn)
	if err != nil {
		_ = tlsConn.Close()
		return nil, err
	}

	resp, err := cc.RoundTrip(req)
	if err != nil {
		if !cc.CanTakeNewRequest() {
			t.evictH2Conn(key, cc)
		}
		return nil, err
	}
	return resp, nil
}

// doHTTP1Request performs an HTTP/1.1 request over the given TLS connection.
func doHTTP1Request(tlsConn *utls.UConn, req *http.Request) (*http.Response, error) {
	transport := &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return tlsConn, nil
		},
		DisableKeepAlives: true,
	}

	return transport.RoundTrip(req)
}

// Compile-time check that chromeTransport implements http.RoundTripper
var _ http.RoundTripper = (*chromeTransport)(nil)

// negotiatedProtocol returns the ALPN protocol used to fetch resp, or
// "http/1.1" as the documented default when the connection didn't surface
// one (plain HTTP, or TLS without ALPN). Returns "" only when the inputs
// are too incomplete to classify (defensive — should not happen on a
// successful client.Do).
func negotiatedProtocol(req *http.Request, resp *http.Response) string {
	if resp != nil && resp.TLS != nil && resp.TLS.NegotiatedProtocol != "" {
		return resp.TLS.NegotiatedProtocol
	}
	if req != nil && req.URL != nil {
		// HTTPS with no ALPN (rare — server didn't advertise any) and plain
		// HTTP both default to HTTP/1.1.
		return "http/1.1"
	}
	return ""
}
