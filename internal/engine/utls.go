// Package portal provides content extraction with SPA detection
package engine

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

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

// chromeTransport implements http.RoundTripper with Chrome TLS fingerprint.
// Handles both HTTP/1.1 and HTTP/2 depending on server ALPN negotiation.
type chromeTransport struct{}

// RoundTrip implements http.RoundTripper
func (t *chromeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// For HTTP (not HTTPS), use default transport
	if req.URL.Scheme != "https" {
		return http.DefaultTransport.RoundTrip(req)
	}

	// Dial with Chrome TLS fingerprint
	tlsConn, err := dialTLSChrome(req.Context(), req.URL.Hostname(), req.URL.Host)
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

	tlsConn := utls.UClient(conn, &utls.Config{
		ServerName: serverName,
	}, utls.HelloChrome_120)

	if err := tlsConn.Handshake(); err != nil {
		_ = conn.Close()
		return nil, err
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
