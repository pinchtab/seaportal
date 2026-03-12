# SeaPortal

Extract clean Markdown from URLs with SPA detection.

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

- **Fast** — Pure HTTP, no browser required (<2s typical)
- **SPA Detection** — Identifies JavaScript-rendered pages
- **Bot Detection Bypass** — TLS fingerprinting, realistic headers
- **Clean Output** — Readability extraction + Markdown conversion
- **Deduplication** — Removes repeated content blocks

## Environment Variables

- `SEAPORTAL_BINARY_PATH` — Custom binary path (for Docker, dev builds)

## License

MIT
