# Security Checklist

## HTTP Requests

- [ ] No user-controlled URLs passed to `os/exec`
- [ ] Request timeouts enforced on all HTTP calls
- [ ] Response body size limits in place
- [ ] No following of unlimited redirects
- [ ] SSRF protections (no fetching private IPs unless explicitly allowed)

## Content Processing

- [ ] HTML parsing handles malformed input without panics
- [ ] No eval or script execution on fetched content
- [ ] Output sanitised — no raw HTML in markdown output
- [ ] Unicode edge cases handled (RTL, zero-width chars)

## Dependencies

- [ ] `go mod tidy` — no unused dependencies
- [ ] Regular `govulncheck` runs
- [ ] gosec scan in CI (configured)

## Release

- [ ] Binaries built with `-s -w` (strip debug info)
- [ ] Checksums published with releases
- [ ] No secrets in binary (version via ldflags only)
