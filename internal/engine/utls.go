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

var (
	utlsClient     *http.Client
	utlsClientOnce sync.Once
)

const h2IdleConnTimeout = 90 * time.Second

const h2DrainTimeout = 5 * time.Second

const maxH2Conns = 256

const (
	dialTimeout          = 30 * time.Second
	dialKeepAlive        = 30 * time.Second
	plainIdleConnTimeout = 90 * time.Second
)

func getUTLSClient() *http.Client {
	utlsClientOnce.Do(func() {
		utlsClient = &http.Client{
			Timeout:   DefaultClientTimeout,
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

func getUTLSClientWithProxy(proxyURL *url.URL) *http.Client {
	return &http.Client{
		Timeout:   DefaultClientTimeout,
		Transport: &chromeTransport{proxyURL: proxyURL},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

type chromeTransport struct {
	proxyURL *url.URL
	security *SecurityPolicy

	tlsConfig *utls.Config

	mu      sync.Mutex
	h2Trans *http2.Transport
	h2Conns map[string]*http2.ClientConn
	h2Last  map[string]time.Time
	plainTr http.RoundTripper
}

func (t *chromeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return t.plainTransport().RoundTrip(req)
	}

	key := canonicalHostPort(req.URL.Host)

	if cc := t.cachedH2Conn(key); cc != nil {
		resp, err := cc.RoundTrip(req)
		if err == nil {
			return withNegotiatedALPN(resp, "h2"), nil
		}
		if cc.CanTakeNewRequest() || req.Context().Err() != nil {
			return nil, err
		}
		t.evictH2Conn(key, cc)
		var ok bool
		if req, ok = rewindRequest(req); !ok {
			return nil, err
		}
	}

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

func (t *chromeTransport) dialTLS(req *http.Request) (*utls.UConn, error) {
	if t.proxyURL != nil {
		return dialTLSChromeViaProxy(req.Context(), t.proxyURL, req.URL.Hostname(), req.URL.Host)
	}
	return dialTLSChrome(req.Context(), req.URL.Hostname(), req.URL.Host, t.security.dialControl(), t.tlsConfig)
}

func (t *chromeTransport) plainTransport() http.RoundTripper {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.plainTr == nil {
		switch {
		case t.proxyURL != nil:
			t.plainTr = &http.Transport{
				Proxy:           http.ProxyURL(t.proxyURL),
				IdleConnTimeout: plainIdleConnTimeout,
			}
		default:
			if ctrl := t.security.dialControl(); ctrl != nil {
				t.plainTr = &http.Transport{
					DialContext: (&net.Dialer{
						Timeout:   dialTimeout,
						KeepAlive: dialKeepAlive,
						Control:   ctrl,
					}).DialContext,
					IdleConnTimeout: plainIdleConnTimeout,
				}
			} else {
				t.plainTr = http.DefaultTransport
			}
		}
	}
	return t.plainTr
}

func (t *chromeTransport) h2Transport() *http2.Transport {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.h2Trans == nil {
		t.h2Trans = &http2.Transport{IdleConnTimeout: h2IdleConnTimeout}
	}
	return t.h2Trans
}

func (t *chromeTransport) cachedH2Conn(key string) *http2.ClientConn {
	t.mu.Lock()
	cc, ok := t.h2Conns[key]
	if ok && !cc.CanTakeNewRequest() {
		delete(t.h2Conns, key)
		delete(t.h2Last, key)
		t.mu.Unlock()
		go drainH2Conn(cc)
		return nil
	}
	if ok {
		t.h2Last[key] = time.Now()
	}
	t.mu.Unlock()
	if !ok {
		return nil
	}
	return cc
}

func (t *chromeTransport) evictH2Conn(key string, cc *http2.ClientConn) {
	t.mu.Lock()
	if t.h2Conns[key] == cc {
		delete(t.h2Conns, key)
		delete(t.h2Last, key)
	}
	t.mu.Unlock()
	go drainH2Conn(cc)
}

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
		go drainH2Conn(existing)
	}
	if t.h2Conns == nil {
		t.h2Conns = make(map[string]*http2.ClientConn)
		t.h2Last = make(map[string]time.Time)
	}
	t.h2Conns[key] = cc
	t.h2Last[key] = time.Now()
	t.evictOverCapLocked()
	t.mu.Unlock()
	return cc, nil
}

func (t *chromeTransport) evictOverCapLocked() {
	for len(t.h2Conns) > maxH2Conns {
		key := t.lruIdleH2KeyLocked()
		if key == "" {
			return
		}
		cc := t.h2Conns[key]
		delete(t.h2Conns, key)
		delete(t.h2Last, key)
		go drainH2Conn(cc)
	}
}

func (t *chromeTransport) lruIdleH2KeyLocked() string {
	var lruKey string
	var lruAt time.Time
	for k, cc := range t.h2Conns {
		if cc.State().StreamsActive > 0 {
			continue
		}
		if at := t.h2Last[k]; lruKey == "" || at.Before(lruAt) {
			lruKey, lruAt = k, at
		}
	}
	return lruKey
}

func (t *chromeTransport) CloseIdleConnections() {
	t.mu.Lock()
	conns := t.h2Conns
	t.h2Conns = nil
	t.h2Last = nil
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

func drainH2Conn(cc *http2.ClientConn) {
	ctx, cancel := context.WithTimeout(context.Background(), h2DrainTimeout)
	defer cancel()
	if err := cc.Shutdown(ctx); err != nil {
		_ = cc.Close()
	}
}

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

func withNegotiatedALPN(resp *http.Response, alpn string) *http.Response {
	if resp.TLS == nil {
		resp.TLS = &tls.ConnectionState{NegotiatedProtocol: alpn}
	}
	return resp
}

func canonicalHostPort(host string) string {
	if _, _, err := net.SplitHostPort(host); err != nil {
		return net.JoinHostPort(host, "443")
	}
	return host
}

func dialTLSChrome(ctx context.Context, serverName, host string, control func(network, address string, c syscall.RawConn) error, tlsOverride *utls.Config) (*utls.UConn, error) {
	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: dialKeepAlive,
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
	if tlsOverride != nil {
		if tlsOverride.InsecureSkipVerify {
			cfg.InsecureSkipVerify = true
		}
		if tlsOverride.RootCAs != nil {
			cfg.RootCAs = tlsOverride.RootCAs
		}
	}
	tlsConn := utls.UClient(conn, cfg, utls.HelloChrome_120)

	if err := tlsConn.Handshake(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return tlsConn, nil
}

func dialTLSChromeViaProxy(ctx context.Context, proxyURL *url.URL, serverName, host string) (*utls.UConn, error) {
	host = canonicalHostPort(host)

	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: dialKeepAlive,
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

func doHTTP1Request(tlsConn *utls.UConn, req *http.Request) (*http.Response, error) {
	transport := &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return tlsConn, nil
		},
		DisableKeepAlives: true,
	}

	return transport.RoundTrip(req)
}

var _ http.RoundTripper = (*chromeTransport)(nil)

func negotiatedProtocol(req *http.Request, resp *http.Response) string {
	if resp != nil && resp.TLS != nil && resp.TLS.NegotiatedProtocol != "" {
		return resp.TLS.NegotiatedProtocol
	}
	if req != nil && req.URL != nil {
		return "http/1.1"
	}
	return ""
}
