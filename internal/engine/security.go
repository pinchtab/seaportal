package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
)

type SecurityPolicy struct {
	BlockPrivateIPs bool

	TrustedResolveCIDRs []string

	AllowedDomains []string
	DeniedDomains  []string

	AllowedSchemes []string

	MaxRedirects int

	RevalidateRedirects bool

	MaxResponseBytes     int64
	MaxDecompressedBytes int64

	Resolver IPResolver

	URLFilter func(ctx context.Context, rawURL string) error `json:"-"`
}

type IPResolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

var (
	ErrSecurityScheme     = errors.New("scheme not allowed by security policy")
	ErrSecurityDomain     = errors.New("host blocked by security policy")
	ErrPrivateIPBlocked   = errors.New("target resolves to a private/internal IP")
	ErrSecurityResolve    = errors.New("could not resolve target host")
	ErrResponseTooLarge   = errors.New("response exceeds the configured size cap")
	ErrDecompressTooLarge = errors.New("decompressed body exceeds the configured size cap")
)

func (p *SecurityPolicy) resolver() IPResolver {
	if p != nil && p.Resolver != nil {
		return p.Resolver
	}
	return net.DefaultResolver
}

func DefaultSecurityPolicy() *SecurityPolicy {
	return &SecurityPolicy{
		BlockPrivateIPs:      true,
		AllowedSchemes:       []string{"http", "https"},
		MaxRedirects:         10,
		RevalidateRedirects:  true,
		MaxResponseBytes:     50 << 20,
		MaxDecompressedBytes: 200 << 20,
	}
}

func (p *SecurityPolicy) ValidateURL(ctx context.Context, rawURL string) error {
	if p == nil {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil
	}
	if u.Scheme != "" {
		if err := p.checkScheme(u.Scheme); err != nil {
			return err
		}
	}
	host := u.Hostname()
	if host == "" {
		return nil
	}
	if err := p.checkDomain(host); err != nil {
		return err
	}
	if err := p.checkResolvedHost(ctx, host); err != nil {
		return err
	}
	if p.URLFilter != nil {
		return p.URLFilter(ctx, rawURL)
	}
	return nil
}

func (p *SecurityPolicy) checkScheme(scheme string) error {
	if len(p.AllowedSchemes) == 0 {
		return nil
	}
	scheme = strings.ToLower(scheme)
	for _, s := range p.AllowedSchemes {
		if strings.ToLower(strings.TrimSpace(s)) == scheme {
			return nil
		}
	}
	return fmt.Errorf("%w: %q", ErrSecurityScheme, scheme)
}

func (p *SecurityPolicy) checkDomain(host string) error {
	host = normalizeSecHost(host)
	if host == "" {
		return nil
	}
	for _, d := range p.DeniedDomains {
		if hostMatchesDomain(host, d) {
			return fmt.Errorf("%w: %q (denied)", ErrSecurityDomain, host)
		}
	}
	if len(p.AllowedDomains) > 0 {
		for _, d := range p.AllowedDomains {
			if hostMatchesDomain(host, d) {
				return nil
			}
		}
		return fmt.Errorf("%w: %q (not in allowlist)", ErrSecurityDomain, host)
	}
	return nil
}

func (p *SecurityPolicy) checkResolvedHost(ctx context.Context, host string) error {
	if !p.BlockPrivateIPs {
		return nil
	}
	host = normalizeSecHost(host)
	if host == "" {
		return ErrSecurityResolve
	}
	trusted := parseCIDRs(p.TrustedResolveCIDRs)
	if ip := net.ParseIP(host); ip != nil {
		return validateIPWithTrusted(ip, trusted)
	}
	ips, err := p.resolver().LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("%w: %s", ErrSecurityResolve, host)
	}
	for _, ip := range ips {
		if err := validateIPWithTrusted(ip, trusted); err != nil {
			return err
		}
	}
	return nil
}

func (p *SecurityPolicy) dialControl() func(network, address string, c syscall.RawConn) error {
	if p == nil || !p.BlockPrivateIPs {
		return nil
	}
	trusted := parseCIDRs(p.TrustedResolveCIDRs)
	return func(network, address string, c syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("%w: %q", ErrPrivateIPBlocked, address)
		}
		return validateIPWithTrusted(ip, trusted)
	}
}

func (p *SecurityPolicy) redirectChecker(tracker *redirectTracker) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		tracker.count++
		if p.MaxRedirects >= 0 && len(via) >= p.MaxRedirects {
			return http.ErrUseLastResponse
		}
		if len(via) > 0 {
			tracker.chain = append(tracker.chain, via[len(via)-1].URL.String())
		}
		if p.RevalidateRedirects {
			if err := p.ValidateURL(req.Context(), req.URL.String()); err != nil {
				return err
			}
		}
		return nil
	}
}

func validateIPWithTrusted(ip net.IP, trusted []*net.IPNet) error {
	if err := validatePublicIP(ip); err != nil {
		if ipInCIDRs(ip, trusted) {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrPrivateIPBlocked, ip)
	}
	return nil
}

var blockedSecPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("198.18.0.0/15"),
}

func validatePublicIP(ip net.IP) error {
	if ip == nil {
		return ErrPrivateIPBlocked
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return ErrPrivateIPBlocked
	}
	addr = addr.Unmap()
	if addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return ErrPrivateIPBlocked
	}
	for _, prefix := range blockedSecPrefixes {
		if prefix.Contains(addr) {
			return ErrPrivateIPBlocked
		}
	}
	return nil
}

func parseCIDRs(raw []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			if ip := net.ParseIP(s); ip != nil && ip.To4() != nil {
				s += "/32"
			} else {
				s += "/128"
			}
		}
		if _, cidr, err := net.ParseCIDR(s); err == nil {
			nets = append(nets, cidr)
		}
	}
	return nets
}

func ipInCIDRs(ip net.IP, cidrs []*net.IPNet) bool {
	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func normalizeSecHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func hostMatchesDomain(host, domain string) bool {
	domain = normalizeSecHost(domain)
	if domain == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func limitedReadAll(r io.Reader, max int64, overflow error) ([]byte, error) {
	if max <= 0 {
		return io.ReadAll(r)
	}
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, overflow
	}
	return data, nil
}
