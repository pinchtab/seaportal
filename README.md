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

# Subcommands
seaportal sitemap https://pinchtab.com/sitemap.xml  # Flatten a sitemap
seaportal feed https://pinchtab.com/feed.xml        # Parse RSS / Atom / JSON Feed
seaportal mcp                                       # Run as an MCP server over stdio

# Version
seaportal --version
```

The full flag list and subcommands are in the [CLI reference](docs/reference/cli.md). SeaPortal also runs as an [MCP server](docs/reference/mcp.md) (`seaportal mcp`), and ships `seabench`, a [benchmark/evaluation harness](docs/reference/seabench.md).

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

The public package is the module root, `github.com/pinchtab/seaportal`:

```go
import "github.com/pinchtab/seaportal"

// Extract content
result := seaportal.FromURL("https://pinchtab.com")
fmt.Println(result.Content) // extracted Markdown

// With options
result := seaportal.FromURLWithOptions("https://pinchtab.com", seaportal.Options{
    Dedupe:   true,
    FastMode: true,
})

// Build accessibility snapshot
snapshot, err := seaportal.BuildSnapshot(htmlString)

// Snapshot with options (filter, max tokens)
opts := seaportal.SnapshotOptions{
    FilterInteractive: true,
    MaxTokens:         2000,
}
snapshot, err := seaportal.BuildSnapshotWithOptions(htmlString, opts)

// Compact text output
fmt.Println(snapshot.ToCompact())
```

See the [API reference](docs/reference/api.md) for the full surface.

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

For JS-heavy pages, use a browser and pass HTML to `seaportal.FromHTML()`.

## License

MIT
