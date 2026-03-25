# Architecture

## Overview

SeaPortal is an HTTP-first content extraction tool designed for AI agents. It fetches web pages using standard HTTP requests (no browser) and extracts clean, structured content.

## Pipeline

```
URL → HTTP Fetch → Bot Detection → Classification → Extraction → Cleanup → Output
```

### Stages

1. **HTTP Fetch** — TLS fingerprint-resistant requests via uTLS
2. **Bot Detection** — Identifies Cloudflare, PerimeterX, Incapsula challenges
3. **Classification** — Categorises pages: static, SSR, hydrated, dynamic, blocked
4. **Extraction** — Readability-based content extraction + HTML-to-Markdown
5. **Cleanup** — Deduplication, preprocessing, quality scoring
6. **Output** — Markdown text, JSON, or accessibility snapshot

## Key Design Decisions

- **No browser dependency** — HTTP-only keeps it fast and lightweight
- **uTLS for stealth** — Mimics real browser TLS fingerprints
- **Classification-first** — Knowing the page type guides extraction strategy
- **Quality scoring** — Every extraction gets a confidence score
- **Accessibility snapshots** — Structured tree output for AI consumption

## Directory Structure

```
cmd/seaportal/     CLI entry point
pkg/portal/        Core extraction engine (public API)
pkg/quality/       Content quality scoring
internal/          Test infrastructure
testdata/          HTML fixtures for testing
tests/e2e/         Docker-based end-to-end tests
```
