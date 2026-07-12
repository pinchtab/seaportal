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

// SecurityPolicy is the opt-in, sane-by-default fetch policy threaded through
// the whole FromURL* path. It guards an extraction against SSRF, redirect-to-
// internal, DNS-rebinding, oversized-body, and decompression-bomb attacks when
// SeaPortal is pointed at attacker-controlled URLs.
//
// Construct a secure default with DefaultSecurityPolicy. A nil *SecurityPolicy
// (the zero value of Options.Security) disables every check and preserves the
// historical "fetch anything" behaviour, so existing callers are unaffected.
//
// The struct is pure configuration with no hidden mutable state, so a single
// policy value may be shared across concurrent FromURL* calls.
type SecurityPolicy struct {
	// BlockPrivateIPs rejects targets that resolve to RFC1918 / loopback /
	// link-local / ULA / unspecified / multicast / carrier-grade-NAT / cloud
	// metadata (169.254.169.254) addresses. Enforced both pre-fetch and at the
	// dial Control hook (post-DNS) to close the DNS-rebinding window.
	BlockPrivateIPs bool

	// TrustedResolveCIDRs are CIDRs (or bare IPs) allowed to resolve to an
	// otherwise-blocked non-public address — the escape hatch for a known
	// internal host or a loopback proxy. Mirrors navguard's TrustedResolveCIDRs.
	TrustedResolveCIDRs []string

	// AllowedDomains, when non-empty, is an allowlist: a target host must equal
	// or be a sub-domain of one entry. DeniedDomains is a blocklist checked
	// first (deny wins). Both match on registrable-suffix (host == d or
	// host endsWith "."+d).
	AllowedDomains []string
	DeniedDomains  []string

	// AllowedSchemes restricts the URL scheme. Empty means "no scheme
	// restriction"; the default is {"http","https"} which rejects file:, ftp:,
	// gopher:, ws:, etc. (data: never reaches the network and is exempt.)
	AllowedSchemes []string

	// MaxRedirects caps redirect hops: >0 = cap at N, 0 = no redirects,
	// -1 = unlimited.
	MaxRedirects int

	// RevalidateRedirects re-runs scheme + domain + resolved-IP validation on
	// every redirect hop, blocking a public→internal redirect SSRF.
	RevalidateRedirects bool

	// MaxResponseBytes caps the raw (still-encoded) response body read off the
	// wire; 0 = unbounded. MaxDecompressedBytes caps decompressor output to
	// stop gzip/br/deflate/zstd decompression bombs; 0 = unbounded.
	MaxResponseBytes     int64
	MaxDecompressedBytes int64

	// Resolver overrides DNS resolution for the BlockPrivateIPs checks; nil
	// uses net.DefaultResolver. Injection seam (T16) — tests supply a
	// map-backed fake so SSRF/rebinding checks stay hermetic; production
	// callers may supply a custom resolver (e.g. DoH). Replaces the former
	// package-global resolveHostIPs hook.
	Resolver IPResolver
}

// IPResolver resolves a hostname to its IP addresses for SecurityPolicy's
// private-IP validation. net.DefaultResolver satisfies it; network is one of
// "ip", "ip4", "ip6" (the policy always asks for "ip" — both families).
type IPResolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

// Security sentinel errors. All are wrapped with target context at the call
// site and surface on Result.Error / Result.SecurityBlock.
var (
	ErrSecurityScheme     = errors.New("scheme not allowed by security policy")
	ErrSecurityDomain     = errors.New("host blocked by security policy")
	ErrPrivateIPBlocked   = errors.New("target resolves to a private/internal IP")
	ErrSecurityResolve    = errors.New("could not resolve target host")
	ErrResponseTooLarge   = errors.New("response exceeds the configured size cap")
	ErrDecompressTooLarge = errors.New("decompressed body exceeds the configured size cap")
)

// resolver returns the policy's DNS resolver: p.Resolver when injected, else
// net.DefaultResolver.
func (p *SecurityPolicy) resolver() IPResolver {
	if p != nil && p.Resolver != nil {
		return p.Resolver
	}
	return net.DefaultResolver
}

// DefaultSecurityPolicy returns the recommended secure-by-default posture:
// block private/internal IPs, http/https only, a 10-redirect cap with per-hop
// revalidation, and 50 MiB raw / 200 MiB decompressed body caps. This is what
// the CLI and the MCP server apply by default; library callers opt in by
// setting Options.Security.
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

// ValidateURL enforces scheme + domain + resolved-IP rules on a single URL.
// It is the pre-fetch gate (and the per-hop redirect gate). A nil policy or an
// unparseable URL is a no-op — the normal fetch path surfaces parse errors so
// the security layer never masks them.
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
	return p.checkResolvedHost(ctx, host)
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

// checkResolvedHost resolves host and rejects it if any resolved address is
// non-public (unless covered by TrustedResolveCIDRs). A literal IP host is
// validated directly without a DNS lookup.
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

// dialControl returns a net.Dialer.Control hook that re-validates the concrete
// post-DNS IP just before the socket connects, closing the DNS-rebinding TOCTOU
// window left open by the pre-fetch resolve. Returns nil (a no-op dialer) when
// the policy doesn't block private IPs.
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

// redirectChecker returns an http.Client.CheckRedirect that enforces
// MaxRedirects and, when RevalidateRedirects is set, re-validates each hop's
// destination. It also feeds the existing redirectTracker so RedirectCount /
// RedirectChain stay populated.
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

// validateIPWithTrusted rejects ip when it is non-public, unless it falls in
// one of the trusted CIDRs.
func validateIPWithTrusted(ip net.IP, trusted []*net.IPNet) error {
	if err := validatePublicIP(ip); err != nil {
		if ipInCIDRs(ip, trusted) {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrPrivateIPBlocked, ip)
	}
	return nil
}

// blockedSecPrefixes are extra ranges treated as non-public: carrier-grade NAT
// (100.64/10) and the benchmarking range (198.18/15).
var blockedSecPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("198.18.0.0/15"),
}

// validatePublicIP returns an error when ip is not a routable public address.
// Ported from PinchTab's netguard.ValidatePublicIP.
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

// parseCIDRs parses CIDR strings and bare IPs into IPNets (bare IPs become /32
// or /128). Blank/unparseable entries are dropped. Ported from navguard.
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

// limitedReadAll reads from r, returning an error if more than max bytes are
// available (max <= 0 means unbounded). It reads one byte past max to detect
// overflow without buffering the whole oversized stream.
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
