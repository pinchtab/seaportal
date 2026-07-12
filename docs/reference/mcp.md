# MCP Server

SeaPortal runs as a [Model Context Protocol](https://modelcontextprotocol.io) server over JSON-RPC 2.0, line-delimited, on stdio:

```bash
seaportal mcp
```

No flags are accepted — configuration flows through tool arguments. The server implements `initialize`, `tools/list`, and `tools/call`.

## Client config

```json
{
  "mcpServers": {
    "seaportal": {
      "command": "seaportal",
      "args": ["mcp"]
    }
  }
}
```

## Tools

Each tool wraps an existing library entry point and returns its JSON-marshalled result as a single text content block.

### `fetch_url`

Fetch a URL and return the extracted Markdown + metadata as JSON (the `Result` struct). Wraps `FromURLWithOptions`.

| Argument | Type | Description |
|----------|------|-------------|
| `url` | string | **required** — URL to fetch |
| `dedupe` | boolean | Enable block dedup (default true) |
| `fast` | boolean | Bail early if a browser is needed |
| `with_links` | boolean | Emit discovered links |
| `with_images` | boolean | Emit discovered images |
| `with_tables` | boolean | Emit structured tables |
| `with_comments` | boolean | Emit user comments |
| `max_tokens` | integer | Approximate output token cap |

### `fetch_snapshot`

Build the accessibility-tree snapshot for a URL (HTTP fetch, then snapshot extraction). Wraps `BuildSnapshotWithOptions`.

| Argument | Type | Description |
|----------|------|-------------|
| `url` | string | **required** |
| `filter` | string | `interactive` to keep only links/buttons/inputs; empty for full tree |
| `max_tokens` | integer | Approximate token cap |
| `allow_internal` | boolean | Allow private/internal IP targets |

### `parse_sitemap`

Fetch and flatten a sitemap.xml (nested `<sitemapindex>` supported, `.gz` auto-decompressed). Returns a JSON array of `{loc, lastmod, changefreq, priority}`. Wraps `FlattenSitemap`.

| Argument | Type | Description |
|----------|------|-------------|
| `url` | string | **required** |
| `max_depth` | integer | Max sitemap-index recursion depth (default 5) |
| `max_urls` | integer | Stop after this many URLs (default 50000) |
| `allow_internal` | boolean | Allow private/internal IP targets |

### `parse_feed`

Fetch and parse an RSS 2.0 / Atom 1.0 / JSON Feed 1.x URL into a unified `{title, link, published, summary, author, guid}` JSON array. Wraps `ParseFeed`.

| Argument | Type | Description |
|----------|------|-------------|
| `url` | string | **required** |
| `max_items` | integer | Stop after this many items (default 200) |
| `allow_internal` | boolean | Allow private/internal IP targets |

### `scrape_site`

Scrape a whole site from a base URL and return a structured `ScrapeResult` JSON
(`site`, `pageGroups`, `pages`, `summary`) designed to be handed to PinchTab for
enrichment. Wraps `ScrapeSite`. Server-side guardrails cap `max_pages` (≤200),
`max_per_pattern` (≤50), and `timeout_seconds` (≤180).

| Argument | Type | Description |
|----------|------|-------------|
| `base_url` | string | **required** |
| `max_pages` | integer | Max total pages (default 50, capped at 200) |
| `max_per_pattern` | integer | Max samples per URL pattern (default 8, capped at 50) |
| `full` | boolean | Disable sampling (fetch all discovered pages, still capped) |
| `include_patterns` | string | Comma-separated globs to include |
| `exclude_patterns` | string | Comma-separated globs to exclude |
| `sample_strategy` | string | `random` \| `priority` \| `balanced` (default balanced) |
| `with_performance` | boolean | Include per-page performance data |
| `respect_robots` | boolean | Respect robots.txt + crawl-delay (default true) |
| `timeout_seconds` | integer | Overall timeout (default 60, capped at 180) |
| `user_agent` | string | Override the User-Agent header |
| `allow_internal` | boolean | Allow private/internal IP targets |

Like `fetch_url`, every scrape fetch (discovery, robots.txt, sitemaps, pages)
runs under the secure-by-default policy; `allow_internal` lifts only the
private-IP block.

## Errors

Tool-level failures return a successful JSON-RPC result with `isError: true` and the error message as text; handler panics are caught and returned the same way.
