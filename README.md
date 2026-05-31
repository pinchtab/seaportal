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

- **Fast on its niche** — Pure HTTP; on reachable static/SSR pages p50 ~1s, p95 ~2s ([across the open web the tail is much longer](#reliability--what-to-expect))
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

> The `quality` float is an **advisory soft signal, not a gate** — clean
> server-rendered pages routinely score ~0 while extracting perfectly. Route on the
> page class and browser-recommendation signal (`profile.decision` /
> `browserRecommended`), not the raw `quality` value. See
> [api.md](docs/reference/api.md) and
> [browser-discriminator.md](docs/reference/browser-discriminator.md).

## Reliability / what to expect

SeaPortal is a **fast first-pass triage that fails over**, not a universal fetcher.
It wins on static and server-rendered pages and tells you when to reach for a browser
instead of pretending every URL extracts.

Numbers below are a frozen snapshot of the committed live sweeps — full breakdown,
dates, and git SHAs in the [reliability reference](docs/reference/reliability.md):

| | Reachable, in-niche (static/SSR) | Across the open web (Tranco top-1000) |
|---|---|---|
| Latency (ok fetches) | p50 ~1s, p95 ~2s | p50 ~1.6s, **p90 >10s, p95 ~15s** |
| Success | ~94% ok | 40% ok — **~53%** netting out the ~242 dead CDN/DNS infra hosts |

What that means in practice:

- In its niche it's fast and reliable.
- Across the raw open web, ~1 in 3 hosts time out and ~1 in 4 error — many are
  CDN/DNS infrastructure domains (`akamaiedge.net`, `cloudfront.net`, …) that never
  serve HTML.
- Treat extraction as triage: set `--timeout` and route on the browser-recommendation
  signal (`profile.decision` / `browserRecommended`), failing over to a real browser
  rather than assuming the happy path.

Regenerate any time with `./dev bench sweep` (see the
[seabench reference](docs/reference/seabench.md)).

## What It Doesn't Do

- JavaScript execution
- Full browser rendering
- Cookie/session management

For JS-heavy pages, use a browser and pass HTML to `seaportal.FromHTML()`.

## Core vs. advanced surfaces

SeaPortal is, first, one thing: a **fast, no-browser fetch-and-extract primitive**
that returns clean Markdown + an accessibility snapshot and tells you when a page
needs a browser. That is the core, and everything in the value prop above describes it.

Layered on top are **secondary, opt-in helpers** — useful, but not the identity and
off by default:

| Surface | What it is | Where |
|---|---|---|
| Chunking (`--chunk`) | Split Markdown into heading/sentence/window chunks for RAG | [api.md](docs/reference/api.md#content-processing) |
| BM25 ranking (`--query`) | Score heading-bounded sections by relevance | [api.md](docs/reference/api.md#content-processing) |
| Split output (`--split-*`) | Shard a large extraction across files | [api.md](docs/reference/api.md#content-processing) |
| TEI-Lite XML (`--xml`) | Wrap a result as TEI-Lite for corpus tooling | [api.md](docs/reference/api.md) |
| Sitemaps & feeds | Flatten `sitemap.xml`, parse RSS/Atom/JSON Feed | [api.md](docs/reference/api.md#sitemaps--feeds) |
| `seabench` | Benchmark / capability harness — **dev tooling, not shipped product** | [seabench.md](docs/reference/seabench.md) |

If you only want the core, ignore all of the above: `seaportal <url>` and
`seaportal.FromURL(...)` never touch them.

## License

MIT
