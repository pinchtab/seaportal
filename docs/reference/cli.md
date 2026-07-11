# CLI Reference

```bash
seaportal [options] <url>          # Extract Markdown / JSON / snapshot (default verb)
seaportal sitemap <url> [flags]    # Flatten a sitemap.xml (recurses sitemap-index)
seaportal feed <url> [flags]       # Parse RSS / Atom / JSON Feed into unified entries
seaportal scrape <base-url> [flags] # Scrape a whole site (discover, sample, extract) → structured JSON
seaportal mcp                      # Run as an MCP server over stdio (see mcp.md)
seaportal help                     # Show usage
seaportal version                  # Show version (also --version / -v)
```

The default verb extracts a URL. It writes the rendered Markdown and JSON to `renders/seaportal/<host>_<timestamp>.{md,json}` and prints the content plus a classification summary. Use `--json`, `--xml`, or `--snapshot` to control the format.

## Input modes

- **URL**: `seaportal https://example.com`
- **stdin HTML**: pipe HTML and pass `--base-url` to resolve relative links:
  ```bash
  cat page.html | seaportal --base-url https://example.com
  ```
  In stdin mode, `--head-only`, `--respect-robots`, and `--retries` are ignored.

## Extract flags

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | Output JSON instead of Markdown |
| `--xml` | false | Output TEI-Lite XML (mutually exclusive with `--json`) |
| `--snapshot` | false | Output accessibility tree (see Snapshot flags) |
| `--fast` | false | Bail early if a browser is needed |
| `--probe-search` | false | Force `needs-browser` for search pages with no result list |
| `--no-dedupe` | false | Disable block deduplication (on by default) |
| `--no-near-dedupe` | false | Disable simhash near-duplicate detection |
| `--max-tokens N` | 0 | Approximate token cap for output (0 = unlimited) |
| `--retries N` | 3 | Retry attempts for 502/503/504/429 |
| `--max-retry-wait D` | 30s | Max single backoff wait |
| `--retry-timeout D` | 90s | Total budget for all retries |
| `--with-links` | false | Emit discovered `<a>` links |
| `--with-images` | false | Emit discovered `<img>` entries |
| `--with-tables` | false | Emit structured tables |
| `--with-comments` | false | Emit user comments separately |
| `--links MODE` | all | Link retention: `none` / `text` / `all` / `footer` |
| `--citations` | false | Numbered references + `## References` (synonym for `--links=footer`) |
| `--chunk SPEC` | "" | `heading` / `sentence[:SIZE]` / `window[:SIZE[:OVERLAP]]` |
| `--select CSS` | "" | CSS selector(s) to scope extraction (comma-separated) |
| `--strip CSS` | "" | CSS selector(s) to remove before extraction |
| `--head-only` | false | Fetch first 16 KB; metadata + canonical only |
| `--no-prune-fallback` | false | Disable tag-density fallback for thin output |
| `--respect-robots` | false | Consult robots.txt and refuse disallowed paths |
| `--rate-limit D` | 0 | Min interval between requests to the same host |
| `--ua VALUE` | "" | UA preset (`chrome`/`safari`/`firefox`/`googlebot`/`bingbot`/`seaportal`/`search-bot`) or literal string |
| `--base-url URL` | "" | Base URL for stdin HTML input |
| `--proxy URL` | "" | `http(s)://` or `socks5://` proxy |
| `--no-pdf` | false | Skip PDF extraction |
| `--schema PATH` | "" | CSS schema (JSON/YAML) → `result.schema` |
| `--version`, `-v` | — | Show version |

### Cache

| Flag | Default | Description |
|------|---------|-------------|
| `--cache DIR` | "" | Enable on-disk cache at this directory |
| `--cache-ttl D` | 24h | Cache freshness window |
| `--cache-stale-tolerance D` | 0 | Stale-while-revalidate window past TTL |
| `--no-cache` | false | Bypass cache reads (writes still happen if `--cache` set) |

### Query / BM25

| Flag | Default | Description |
|------|---------|-------------|
| `--query TEXT` | "" | Score sections by BM25 relevance |
| `--top-n N` | 0 | Keep only the top-N most relevant sections |
| `--filter-by-query` | false | Replace content with concatenated top-N sections (default top-3) |

### Split output

| Flag | Default | Description |
|------|---------|-------------|
| `--split-out DIR` | "" | Write split output files into DIR (not supported with `--xml`) |
| `--split-bytes N` | 0 | Approx bytes per file (default `--max-tokens × 4` or 32768) |

### Security

The CLI is **safe by default**: it applies `DefaultSecurityPolicy()` (SSRF /
private-IP block on, `http`/`https` only, redirect + body caps). Loosen it only
for trusted targets — e.g. `--allow-internal` to reach `localhost` / a private
host. See [SECURITY.md](../../SECURITY.md) for the threat model and coverage
caveats (`--proxy` skips the dial-time rebinding check).

| Flag | Default | Description |
|------|---------|-------------|
| `--block-private-ips` | true | Reject targets resolving to private/internal IPs (SSRF guard) |
| `--allow-internal` (alias `--allow-private-ips`) | false | Escape hatch: allow private/internal IP targets |
| `--max-redirects N` | 10 | Max redirect hops (`0` = none, `-1` = unlimited); each hop is re-validated |
| `--allow-domains LIST` | "" | Comma-separated host allowlist (suffix match); empty = allow any |
| `--deny-domains LIST` | "" | Comma-separated host blocklist (suffix match; deny wins) |
| `--trusted-resolve-cidrs LIST` | "" | CIDRs/IPs allowed to resolve to non-public addresses |
| `--max-response-bytes N` | 52428800 | Max raw response body bytes (0 = unlimited) |
| `--max-decompressed-bytes N` | 209715200 | Max decompressed body bytes — defuses decompression bombs (0 = unlimited) |

### Snapshot flags

| Flag | Default | Description |
|------|---------|-------------|
| `--snapshot` | false | Build the accessibility tree |
| `--filter interactive` | "" | Keep only interactive elements |
| `--format json\|compact` | json | JSON tree or readable text tree |
| `--max-tokens N` | 0 | Approximate token cap for the tree |

## `seaportal sitemap`

```bash
seaportal sitemap https://example.com/sitemap.xml [--json] [--max-urls N] [--max-depth N]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | JSON array instead of newline-separated URLs |
| `--max-urls N` | 50000 | Stop after this many URLs |
| `--max-depth N` | 5 | Max sitemap-index recursion depth |

Recurses nested `<sitemapindex>` references and auto-decompresses `.gz` sitemaps.

## `seaportal feed`

```bash
seaportal feed https://example.com/feed.xml [--json] [--max-items N]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | JSON array instead of TSV (`published\ttitle\tlink`) |
| `--max-items N` | 200 | Stop after this many items |

Parses RSS 2.0, Atom 1.0, and JSON Feed 1.x into unified entries.

## `seaportal scrape`

Scrape a whole website from a base URL: discover candidate URLs (robots.txt +
sitemap, or a bounded crawl fallback), cluster them into pattern groups, sample
within budget, fetch and extract each page concurrently, and emit a structured
result. The JSON output is designed to be handed to [PinchTab](https://pinchtab.com)
for deep browser enrichment (screenshots, console/network capture, visual
regression, a11y checks).

```bash
seaportal scrape https://example.com --max-pages 50 --sample-strategy balanced
```

| Flag | Default | Description |
|------|---------|-------------|
| `--max-pages N` | 50 | Maximum total pages to process |
| `--max-per-pattern N` | 8 | Max samples per URL pattern group |
| `--full` | false | Disable per-pattern sampling — take every discovered page, still capped at `--max-pages` |
| `--include-patterns S` | "" | Comma-separated globs to include (`*` within a segment, `**` across) |
| `--exclude-patterns S` | "" | Comma-separated globs to exclude |
| `--sample-strategy S` | balanced | `random` \| `priority` \| `balanced` |
| `--output S` | json | `json` \| `md` \| `directory` |
| `--json` | false | Shorthand for `--output json` (matches the root command) |
| `--out-dir PATH` | "" | Target directory (required for `--output directory`) |
| `--with-performance` | false | Include per-page performance data (TTFB, bytes, requests) |
| `--respect-robots` | true | Honor robots.txt disallow rules + crawl-delay |
| `--timeout D` | 60s | Overall scrape timeout |
| `--user-agent S` | "" | Override the User-Agent header |

Sampling strategies: `balanced` spreads the budget across groups so a large
group can't starve the others; `priority` takes the homepage and one
representative per top-level section first; `random` samples uniformly within
each group (seeded per-site for reproducibility). `--full` turns off the
per-pattern cap but `--max-pages` remains the overall budget, so to fetch every
page of a large site raise `--max-pages` accordingly (e.g. `--full --max-pages
1000`). `--output directory` writes `result.json`, one `pages/<slug>.md` per
page, and an `index.md` manifest.

Sample JSON output (abridged):

```json
{
  "site": {
    "baseURL": "https://example.com",
    "title": "Example Site",
    "sitemapFound": true,
    "totalURLsInSitemap": 12450,
    "sampledPages": 42
  },
  "pageGroups": [
    { "pattern": "/blog/*", "totalInSitemap": 8200, "sampled": 8, "pages": [] }
  ],
  "pages": [
    {
      "url": "https://example.com/blog/hello",
      "title": "Hello",
      "status": 200,
      "meta": { "title": "Hello", "ogType": "article" },
      "markdown": "# Hello\n\n…",
      "schema": [ { "type": "Article" } ],
      "contentType": "article",
      "internalLinks": 45,
      "externalLinks": 12
    }
  ],
  "summary": {
    "contentTypes": { "article": 30, "product": 10, "page": 2 },
    "recommendations": ["sitemap lists 12450 URLs across 6 patterns but only 42 were sampled; sampling recommended"]
  }
}
```
