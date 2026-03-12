# SeaPortal

Fast content extraction for AI agents. HTTP-first, no browser required.

## Install

```bash
# npm (recommended)
npm install -g seaportal

# Go
go install github.com/pinchtab/seaportal/cmd/seaportal@latest
```

## Usage

```bash
seaportal https://pinchtab.com

# Options
seaportal --json https://pinchtab.com       # JSON output
seaportal --snapshot https://pinchtab.com   # Accessibility tree
seaportal --fast https://pinchtab.com       # Bail early if browser needed
seaportal --no-dedupe https://pinchtab.com  # Disable deduplication

# Version
seaportal --version
```

## Accessibility Snapshot

The `--snapshot` flag outputs a semantic accessibility tree — useful for AI agents that need to understand page structure and interact with elements:

```bash
seaportal --snapshot https://pinchtab.com
```

```json
{
  "role": "document",
  "children": [
    {
      "role": "navigation",
      "name": "Main",
      "ref": "e1",
      "children": [
        {"role": "link", "name": "Home", "ref": "e2", "href": "/", "interactive": true},
        {"role": "link", "name": "Docs", "ref": "e3", "href": "/docs", "interactive": true}
      ]
    },
    {
      "role": "main",
      "ref": "e4",
      "children": [
        {"role": "heading", "name": "Welcome", "ref": "e5", "level": 1},
        {"role": "button", "name": "Get Started", "ref": "e6", "interactive": true}
      ]
    }
  ]
}
```

Each node includes:
- **role** — Accessibility role (heading, link, button, textbox, etc.)
- **name** — Accessible name (from aria-label, title, alt, or text)
- **ref** — Element reference (e1, e2...) for targeting
- **interactive** — Whether the element can be clicked/typed
- **level** — Heading level (1-6) for headings
- **href** — Link target for links

## As a Library

```go
import "github.com/pinchtab/seaportal/pkg/portal"

// Extract content
result := portal.FromURL("https://pinchtab.com")

// With options
result := portal.FromURLWithOptions("https://pinchtab.com", portal.Options{
    Dedupe:   true,
    FastMode: true,
})

// Build accessibility snapshot
snapshot, err := portal.BuildSnapshot(htmlString)
```

## Features

- **Fast** — Pure HTTP, typically <2s per extraction
- **Stealthy** — Chrome TLS fingerprint, realistic headers
- **Smart** — Readability extraction + Markdown conversion
- **Semantic** — Accessibility tree for AI agents
- **Honest** — Classifies pages, signals when browser is needed
- **Clean** — Deduplicates repeated content blocks

## Detection

Automatically detects:
- Bot protection (Cloudflare, AWS WAF, DataDome, PerimeterX)
- Captcha pages
- Access denied / login walls
- SPA / JavaScript-only content

## Page Classification

| Class | Description |
|-------|-------------|
| `static` | Pure HTML, high confidence |
| `ssr` | Server-rendered, good extraction |
| `hydrated` | SSR + JS enhancement, usually extractable |
| `spa` | JavaScript-only content, needs browser |
| `dynamic` | Heavy client-side rendering |
| `blocked` | Bot protection, captcha, access denied |

## What It Doesn't Do

- JavaScript execution
- Full browser rendering
- Cookie/session management

For JS-heavy pages, use a browser and pass HTML to `portal.FromHTML()`.

## License

MIT
