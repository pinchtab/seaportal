// Package portal provides content extraction with SPA detection
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
type chromeTransport struct {
	proxyURL *url.URL
}

// RoundTrip implements http.RoundTripper
func (t *chromeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// For HTTP (not HTTPS) — proxy via vanilla transport when configured,
	// else fall through to the default transport.
	if req.URL.Scheme != "https" {
		if t.proxyURL != nil {
			tr := &http.Transport{Proxy: http.ProxyURL(t.proxyURL)}
			return tr.RoundTrip(req)
		}
		return http.DefaultTransport.RoundTrip(req)
	}

	// HTTPS: dial directly or through a CONNECT tunnel, then upgrade to
	// the utls Chrome fingerprint.
	var (
		tlsConn *utls.UConn
		err     error
	)
	if t.proxyURL != nil {
		tlsConn, err = dialTLSChromeViaProxy(req.Context(), t.proxyURL, req.URL.Hostname(), req.URL.Host)
	} else {
		tlsConn, err = dialTLSChrome(req.Context(), req.URL.Hostname(), req.URL.Host)
	}
	if err != nil {
		return nil, err
	}

	// Check ALPN negotiated protocol
	alpn := tlsConn.ConnectionState().NegotiatedProtocol

	var resp *http.Response
	if alpn == "h2" {
		// HTTP/2
		resp, err = doHTTP2Request(tlsConn, req)
	} else {
		// HTTP/1.1
		resp, err = doHTTP1Request(tlsConn, req)
	}

	if err != nil {
		_ = tlsConn.Close()
		return nil, err
	}

	// Surface the negotiated ALPN protocol on resp.TLS so callers can read
	// `resp.TLS.NegotiatedProtocol` exactly as they would with the standard
	// transport. Only synthesise when the response doesn't already carry one
	// (h2 path attaches its own ConnectionState).
	if resp.TLS == nil {
		resp.TLS = &tls.ConnectionState{NegotiatedProtocol: alpn}
	}

	return resp, nil
}

// dialTLSChrome establishes a TLS connection impersonating Chrome 120.
func dialTLSChrome(ctx context.Context, serverName, host string) (*utls.UConn, error) {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// Ensure host has port
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "443")
	}

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
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "443")
	}

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

	// Build CONNECT request.
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

// doHTTP2Request performs an HTTP/2 request over the given TLS connection.
func doHTTP2Request(tlsConn *utls.UConn, req *http.Request) (*http.Response, error) {
	h2Transport := &http2.Transport{}
	h2Conn, err := h2Transport.NewClientConn(tlsConn)
	if err != nil {
		return nil, err
	}

	return h2Conn.RoundTrip(req)
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
