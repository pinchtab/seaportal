# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please report it privately:

1. **Do not** open a public issue
2. Email: security@luigiagosti.com (or open a private security advisory on GitHub)
3. Include details about the vulnerability and steps to reproduce

We will respond within 48 hours and work with you to understand and fix the issue.

## Scope

SeaPortal fetches **attacker-controlled URLs** (and, in stdin mode,
attacker-controlled HTML), so any service that calls it on untrusted input
inherits a server-side request surface. The fetch path defends against:

- **SSRF / private-IP access** — targets that resolve to RFC1918, loopback,
  link-local, ULA, carrier-grade-NAT, unspecified/multicast, or the cloud
  metadata endpoint (`169.254.169.254`) are rejected. The check runs both
  pre-fetch (on the resolved IP) and at the dial Control hook (the concrete
  post-DNS IP), which closes the **DNS-rebinding** TOCTOU window.
- **Redirect-to-internal SSRF** — every redirect hop is re-validated
  (scheme + host + resolved IP) when `RevalidateRedirects` is set, so a public
  URL cannot bounce to an internal one. Hops are capped by `MaxRedirects`.
- **Scheme abuse** — only `http` / `https` are dialled by default; `file:`,
  `ftp:`, `gopher:`, `ws:`, etc. are rejected. (`data:` is decoded in-process
  and never touches the network.)
- **Decompression bombs & oversized bodies** — the raw response and the
  decompressor output (gzip/br/deflate/zstd) are each bounded
  (`MaxResponseBytes` / `MaxDecompressedBytes`), failing closed instead of OOM.
- **Domain allow/deny** — optional allowlist / blocklist on the target host.
- **Robustness** — malformed URLs and malicious HTML must not crash or hang;
  timeouts and retries are bounded.

These are governed by an opt-in `SecurityPolicy` (see
[api.md](docs/reference/api.md) and [cli.md](docs/reference/cli.md)).
`DefaultSecurityPolicy()` is the sane-by-default posture and is applied
automatically by the **CLI** and the **MCP server** (the most exposed
entrypoint). Library callers via `seaportal.FromURL*` opt in by setting
`Options.Security` — a `nil` policy preserves the historical unguarded
behaviour, so embedding code must set a policy when handling untrusted URLs.

### A note on uTLS / "stealth"

SeaPortal can present a Chrome TLS fingerprint (uTLS) to reach sites that
fingerprint Go's TLS stack. This is for reachability, not deception of
operators: the cooperative, well-behaved mode is `--ua=seaportal` plus
`--respect-robots`, which identifies the fetcher and honours `robots.txt`.
Choose the fingerprint posture deliberately for your use case.

### Coverage caveats

- **`--proxy` / `Options.Proxy`** — the dial-time IP re-check (the DNS-rebinding
  guard) only runs on the direct path, since a proxied request dials the proxy,
  not the target. The target host is still vetted pre-fetch and on every redirect
  (scheme + domain + resolved IP), just not at connect time.
- **`--snapshot`** (CLI) — the snapshot-from-URL path uses a lighter HTTP client.
  It pre-validates scheme + host + resolved IP, but does **not** apply the
  redirect re-validation or the response/decompression size caps. Prefer the main
  extract path for untrusted input.
- **Library default is `nil` = unguarded** — see above; only the CLI and MCP are
  safe by default.

### Defaults (`DefaultSecurityPolicy`)

| Knob | Default |
|---|---|
| `BlockPrivateIPs` | `true` |
| `AllowedSchemes` | `http`, `https` |
| `MaxRedirects` | `10` (`0` = none, `-1` = unlimited) |
| `RevalidateRedirects` | `true` |
| `MaxResponseBytes` | 50 MiB (`0` = unlimited) |
| `MaxDecompressedBytes` | 200 MiB (`0` = unlimited) |
| `AllowedDomains` / `DeniedDomains` / `TrustedResolveCIDRs` | empty |

## Supported Versions

Only the latest release is supported with security updates.
