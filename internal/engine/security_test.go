package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type resolverFunc func(ctx context.Context, network, host string) ([]net.IP, error)

func (f resolverFunc) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	return f(ctx, network, host)
}

func stubResolver(m map[string][]net.IP) IPResolver {
	return resolverFunc(func(_ context.Context, _ string, host string) ([]net.IP, error) {
		if ips, ok := m[host]; ok {
			return ips, nil
		}
		return nil, fmt.Errorf("no stub for %q", host)
	})
}

func TestValidatePublicIP_BlocksNonPublic(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"10.0.0.5",
		"192.168.1.1",
		"172.16.0.1",
		"169.254.169.254",
		"100.64.0.1",
		"198.18.0.1",
		"0.0.0.0",
		"::1",
		"fe80::1",
		"fc00::1",
	}
	for _, s := range blocked {
		if err := validatePublicIP(net.ParseIP(s)); err == nil {
			t.Errorf("validatePublicIP(%s) = nil, want blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946"} {
		if err := validatePublicIP(net.ParseIP(s)); err != nil {
			t.Errorf("validatePublicIP(%s) = %v, want allowed", s, err)
		}
	}
}

func TestValidateURL_BlocksPrivateResolution(t *testing.T) {
	p := DefaultSecurityPolicy()
	p.Resolver = stubResolver(map[string][]net.IP{
		"internal.example.com": {net.ParseIP("10.0.0.7")},
		"meta.example.com":     {net.ParseIP("169.254.169.254")},
		"mixed.example.com":    {net.ParseIP("8.8.8.8"), net.ParseIP("127.0.0.1")},
		"public.example.com":   {net.ParseIP("93.184.216.34")},
	})

	for _, host := range []string{"internal.example.com", "meta.example.com", "mixed.example.com"} {
		err := p.ValidateURL(context.Background(), "https://"+host+"/x")
		if !errors.Is(err, ErrPrivateIPBlocked) {
			t.Errorf("ValidateURL(%s) = %v, want ErrPrivateIPBlocked", host, err)
		}
	}
	if err := p.ValidateURL(context.Background(), "https://public.example.com/x"); err != nil {
		t.Errorf("ValidateURL(public) = %v, want nil", err)
	}
	if err := p.ValidateURL(context.Background(), "https://10.1.2.3/x"); !errors.Is(err, ErrPrivateIPBlocked) {
		t.Errorf("ValidateURL(literal 10.1.2.3) = %v, want blocked", err)
	}
}

func TestValidateURL_TrustedResolveCIDRAllowsInternal(t *testing.T) {
	p := DefaultSecurityPolicy()
	p.Resolver = stubResolver(map[string][]net.IP{
		"db.internal": {net.ParseIP("10.0.0.7")},
	})
	p.TrustedResolveCIDRs = []string{"10.0.0.0/8"}
	if err := p.ValidateURL(context.Background(), "https://db.internal/x"); err != nil {
		t.Errorf("with trusted CIDR 10.0.0.0/8, ValidateURL = %v, want nil", err)
	}
	p2 := DefaultSecurityPolicy()
	p2.TrustedResolveCIDRs = []string{"127.0.0.1"}
	if err := p2.ValidateURL(context.Background(), "https://127.0.0.1:8080/x"); err != nil {
		t.Errorf("with trusted 127.0.0.1, ValidateURL = %v, want nil", err)
	}
}

func TestValidateURL_SchemeAndDomainRules(t *testing.T) {
	p := DefaultSecurityPolicy()
	for _, raw := range []string{"file:///etc/passwd", "ftp://host/x", "gopher://host", "ws://host/x"} {
		if err := p.ValidateURL(context.Background(), raw); !errors.Is(err, ErrSecurityScheme) {
			t.Errorf("ValidateURL(%s) = %v, want ErrSecurityScheme", raw, err)
		}
	}

	resolver := stubResolver(map[string][]net.IP{"evil.com": {net.ParseIP("8.8.8.8")}, "ok.com": {net.ParseIP("8.8.8.8")}})
	pd := DefaultSecurityPolicy()
	pd.Resolver = resolver
	pd.DeniedDomains = []string{"evil.com"}
	if err := pd.ValidateURL(context.Background(), "https://sub.evil.com/x"); !errors.Is(err, ErrSecurityDomain) {
		t.Errorf("denied domain not blocked: %v", err)
	}
	if err := pd.ValidateURL(context.Background(), "https://ok.com/x"); err != nil {
		t.Errorf("ok.com should pass deny-only policy: %v", err)
	}

	pa := DefaultSecurityPolicy()
	pa.Resolver = resolver
	pa.AllowedDomains = []string{"ok.com"}
	if err := pa.ValidateURL(context.Background(), "https://evil.com/x"); !errors.Is(err, ErrSecurityDomain) {
		t.Errorf("host outside allowlist not blocked: %v", err)
	}
	if err := pa.ValidateURL(context.Background(), "https://ok.com/x"); err != nil {
		t.Errorf("allowlisted host blocked: %v", err)
	}
}

func TestRedirectChecker_MaxRedirects(t *testing.T) {
	mk := func() *http.Request {
		r, _ := http.NewRequest("GET", "https://example.com/x", nil)
		return r
	}
	mkVia := func(n int) []*http.Request {
		v := make([]*http.Request, n)
		for i := range v {
			v[i] = mk()
		}
		return v
	}
	cases := []struct {
		name     string
		max      int
		viaLen   int
		wantStop bool
	}{
		{"none-blocks-first", 0, 0, true},
		{"cap-2-allows-1", 2, 1, false},
		{"cap-2-blocks-2", 2, 2, true},
		{"unlimited", -1, 50, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &SecurityPolicy{MaxRedirects: c.max}
			check := p.redirectChecker(&redirectTracker{})
			err := check(mk(), mkVia(c.viaLen))
			stopped := errors.Is(err, http.ErrUseLastResponse)
			if stopped != c.wantStop {
				t.Errorf("max=%d viaLen=%d: stopped=%v want=%v (err=%v)", c.max, c.viaLen, stopped, c.wantStop, err)
			}
		})
	}
}

func TestRedirectChecker_RevalidatesToInternal(t *testing.T) {
	p := DefaultSecurityPolicy()
	p.Resolver = stubResolver(map[string][]net.IP{"evil-redirect.com": {net.ParseIP("10.0.0.9")}})
	check := p.redirectChecker(&redirectTracker{})
	req, _ := http.NewRequest("GET", "https://evil-redirect.com/internal", nil)
	via, _ := http.NewRequest("GET", "https://start.example.com/", nil)
	err := check(req, []*http.Request{via})
	if !errors.Is(err, ErrPrivateIPBlocked) {
		t.Fatalf("public→internal redirect not blocked: %v", err)
	}
}

type cannedRT struct {
	status int
	header http.Header
	body   []byte
}

func (rt *cannedRT) RoundTrip(req *http.Request) (*http.Response, error) {
	h := rt.header
	if h == nil {
		h = http.Header{}
	}
	return &http.Response{
		StatusCode: rt.status,
		Status:     fmt.Sprintf("%d %s", rt.status, http.StatusText(rt.status)),
		Header:     h,
		Body:       io.NopCloser(bytes.NewReader(rt.body)),
		Request:    req,
	}, nil
}

func TestFetch_ResponseSizeCapped(t *testing.T) {
	big := bytes.Repeat([]byte("<p>spam</p>"), 300_000)
	rt := &cannedRT{status: 200, header: http.Header{"Content-Type": {"text/html"}}, body: big}
	opts := Options{
		Transport: rt,
		Security:  &SecurityPolicy{MaxResponseBytes: 1 << 20},
	}
	res := FromURLWithOptions("https://example.com/big", opts)
	if !strings.Contains(res.Error, "size cap") || res.SecurityBlock == "" {
		t.Fatalf("oversized body not capped: error=%q securityBlock=%q", res.Error, res.SecurityBlock)
	}
}

func TestFetch_DecompressionBombCapped(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(bytes.Repeat([]byte{'A'}, 20<<20)); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()

	rt := &cannedRT{
		status: 200,
		header: http.Header{"Content-Type": {"text/html"}, "Content-Encoding": {"gzip"}},
		body:   buf.Bytes(),
	}
	opts := Options{
		Transport: rt,
		Security:  &SecurityPolicy{MaxDecompressedBytes: 1 << 20},
	}
	res := FromURLWithOptions("https://example.com/bomb", opts)
	if !strings.Contains(res.Error, "decompressed body exceeds") {
		t.Fatalf("decompression bomb not capped: error=%q", res.Error)
	}
	if res.SecurityBlock == "" {
		t.Fatalf("decompression bomb should set SecurityBlock, got empty")
	}
}

func TestFetch_NoPolicyIsUnguarded(t *testing.T) {
	rt := &cannedRT{status: 200, header: http.Header{"Content-Type": {"text/html"}}, body: []byte("<html><body><h1>hi</h1><p>world</p></body></html>")}
	res := FromURLWithOptions("https://example.com/ok", Options{Transport: rt})
	if res.Error != "" {
		t.Fatalf("unguarded fetch errored: %q", res.Error)
	}
	if res.SecurityBlock != "" {
		t.Fatalf("unguarded fetch set SecurityBlock: %q", res.SecurityBlock)
	}
}
