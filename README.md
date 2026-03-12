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
      "tag": "nav",
      "ref": "e1",
      "selector": "#main-nav",
      "depth": 0,
      "children": [
        {"role": "link", "name": "Home", "tag": "a", "ref": "e2", "selector": "a.nav-link", "depth": 1, "href": "/", "interactive": true}
      ]
    }
  ]
}
```

Each node includes:
- **role** — Accessibility role (heading, link, button, textbox, etc.)
- **name** — Accessible name (from aria-label, title, alt, or text)
- **tag** — HTML tag name (div, a, button, etc.)
- **ref** — Element reference (e1, e2...) for targeting
- **selector** — CSS selector for the element
- **depth** — Nesting depth in the tree
- **interactive** — Whether the element can be clicked/typed
- **level** — Heading level (1-6) for headings
- **href** — Link target for links

### Snapshot Options

```bash
# Filter to interactive elements only
seaportal --snapshot --filter=interactive https://example.com

# Compact text output (instead of JSON)
seaportal --snapshot --format=compact https://example.com

# Limit output size (approximate token count)
seaportal --snapshot --max-tokens=2000 https://example.com

# Combine options
seaportal --snapshot --filter=interactive --format=compact https://example.com
```

**Compact format** outputs a readable text tree:
```
document
  e1 navigation "Main" <nav> [interactive]
    e2 link "Home" <a> [interactive] href=/
    e3 link "Docs" <a> [interactive] href=/docs
  e4 main <main>
    e5 heading "Welcome" <h1> level=1
```

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

// Snapshot with options (filter, max tokens)
opts := portal.SnapshotOptions{
    FilterInteractive: true,
    MaxTokens:         2000,
}
snapshot, err := portal.BuildSnapshotWithOptions(htmlString, opts)

// Compact text output
fmt.Println(snapshot.ToCompact())
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
