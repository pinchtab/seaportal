# SeaPortal

HTTP-first web content extraction for AI agents. Turn static and server-rendered
pages into clean Markdown or a JSON accessibility snapshot, parse sitemaps and
feeds, and run it as a CLI, a Go library, or an MCP server — secure by default,
with an explicit signal when a page actually needs a browser.

## Install

```bash
npm install -g seaportal
# or
npx seaportal <url>
```

## Usage

```bash
# Basic extraction
seaportal https://pinchtab.com

# JSON output
seaportal --json https://pinchtab.com

# Fast mode (bail early if browser needed)
seaportal --fast https://pinchtab.com

# Disable deduplication
seaportal --no-dedupe https://pinchtab.com

# Combine options
seaportal --json --fast https://pinchtab.com
```

## Output

SeaPortal outputs Markdown with YAML frontmatter containing metadata:

```yaml
---
title: "Page Title"
url: https://pinchtab.com
confidence: 85
isSpa: false
needsBrowser: false
---

# Page Title

Content extracted as clean Markdown...
```

## Features

- **Fast** — Pure HTTP, no browser required (<2s typical on static / SSR pages)
- **Clean Markdown** — Readability extraction + block and simhash dedupe
- **Accessibility snapshots** — JSON semantic tree with role, ref, and selector per node
- **Browser-needed signal** — Flags SPA / blocked pages so you can route them elsewhere
- **Sitemaps & feeds** — Flatten sitemap.xml and parse RSS, Atom, and JSON Feed
- **MCP server** — `seaportal mcp` exposes `fetch_url`, `fetch_snapshot`, `parse_sitemap`, and `parse_feed` over stdio
- **Safe by default** — SSRF / private-IP blocking, http(s)-only, redirect and body caps on the CLI and MCP server

## Environment Variables

- `SEAPORTAL_BINARY_PATH` — Custom binary path (for Docker, dev builds)

## License

MIT
