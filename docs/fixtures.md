# FIXTURES.md — SeaPortal Test Fixtures

## Overview

SeaPortal uses two testing modes:
1. **Live testing** (`go run ./cmd/test`) — fetches URLs in real-time, compares HTTP extraction vs browser
2. **Fixture testing** — uses saved HTML snapshots in `testdata/` for deterministic, offline tests

## Fixture Categories

Fixtures are organized by page classification:

| Category   | Path                  | Purpose                              |
|------------|-----------------------|--------------------------------------|
| `static`   | `testdata/static/`    | Plain HTML, no JS needed             |
| `ssr`      | `testdata/ssr/`       | Server-rendered with JS enhancement  |
| `hydrated` | `testdata/hydrated/`  | SSR + significant client hydration   |
| `dynamic`  | `testdata/dynamic/`   | Content varies significantly         |
| `spa`      | `testdata/spa/`       | JS-only rendering, needs browser     |
| `blocked`  | `testdata/blocked/`   | Challenge pages (Cloudflare, captcha)|

## Hydrated Fixture Limitation

**Important:** Hydrated fixtures capture the *initial HTML response* only. They do NOT capture:
- Client-side hydration effects
- Lazy-loaded content
- JS-rendered components
- Personalized/A/B tested content

This means fidelity comparisons against hydrated fixtures may differ from live tests where the browser fully renders the page. Hydrated fixtures are best used for:
- Testing extraction of SSR-provided content
- Verifying classification logic
- Regression testing for parser changes

They should NOT be used to validate fidelity against browser output.

## Adding New Fixtures

```bash
# Save a fixture snapshot
curl -s https://pinchtab.com > testdata/static/example.html

# Or use the snapshot flag during testing
go run ./cmd/test --snapshot
```

## Current Fixture Corpus

### blocked/
- `cloudflare-challenge.html` — Cloudflare "Just a moment" page
- `captcha.html` — Generic captcha page
- `rate-limit.html` — Rate limiting response
- `access-denied.html` — Access denied page

### static/
- `example.html` — example.com
- `cern.html` — CERN first website

### ssr/
- (expand with go.dev, MDN, blog.golang.org snapshots)

### hydrated/
- (expand with github.com, wikipedia snapshots)

### dynamic/
- (expand with HN, BBC snapshots)

## Blocked Detection

The blocked detector uses a two-tier approach:
1. **Head-level patterns** (always checked): title indicators, Cloudflare JS variables
2. **Body-level patterns** (only on short pages): captcha, bot detection, access denied keywords

Body patterns are checked against stripped text (scripts/styles removed) to avoid false positives from sites that mention "captcha" in their JS configs (e.g., Reddit, Airbnb).
