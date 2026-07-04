# API Reference

The public Go API is the **module-root package `seaportal`** (`github.com/pinchtab/seaportal`). All implementation lives under `internal/`; this package re-exports the stable surface.

```go
import "github.com/pinchtab/seaportal"
```

## Extraction

```go
// Default options.
result := seaportal.FromURL("https://example.com")
fmt.Println(result.Content) // extracted Markdown body

// Custom options.
result := seaportal.FromURLWithOptions("https://example.com", seaportal.Options{
    FastMode:  true,
    WithLinks: true,
})

// From raw HTML you already have (e.g. fetched by a browser).
result := seaportal.FromHTML(htmlString, "https://example.com")
result := seaportal.FromHTMLWithOptions(htmlString, "https://example.com", opts)
```

| Function | Signature |
|----------|-----------|
| `FromURL` | `func(targetURL string) Result` |
| `FromURLWithOptions` | `func(targetURL string, opts Options) Result` |
| `FromURLWithDedupe` | `func(targetURL string) Result` |
| `FromHTML` | `func(html, targetURL string) Result` |
| `FromHTMLWithOptions` | `func(html, targetURL string, opts Options) Result` |
| `FromResponse` | `func(resp *http.Response, targetURL string, start time.Time) Result` |
| `ExtractFromHTML` | `func(html, targetURL string) (string, error)` — Markdown only |
| `ResultToTEIXML` | `func(r Result) ([]byte, error)` — TEI-Lite XML |

Extraction functions return `Result` by value and never error; transport and parse failures are reported on `Result.Error`, `Result.StatusCode`, and `Result.Profile`.

## `Options`

Selected fields (see `seaportal.go` / `internal/engine` for the full struct):

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Dedupe` | bool | false | Remove duplicate content blocks |
| `NoNearDedupe` | bool | false | Disable simhash near-duplicate detection |
| `FastMode` | bool | false | Bail early if a browser is likely needed |
| `ProbeSearch` | bool | false | Force `needs-browser` for search pages with no result list |
| `MaxRetries` | int | — | Retry attempts for 502/503/504/429 |
| `MaxRetryWait` | time.Duration | — | Max single backoff wait |
| `TotalRetryTimeout` | time.Duration | — | Total budget across retries |
| `WithLinks` | bool | false | Emit discovered `<a>` links on `Result` |
| `WithImages` | bool | false | Emit discovered `<img>` entries |
| `WithTables` | bool | false | Emit structured tables |
| `WithComments` | bool | false | Emit user comments on `Result.Comments` |
| `Citations` | bool | false | Numbered references + `## References` (alias for `LinkRetention = footer`) |
| `LinkRetention` | LinkRetention | `LinkRetentionAll` | `none` / `text` / `all` / `footer` |
| `Chunk` | ChunkConfig | off | Markdown chunking strategy |
| `SelectCSS` / `StripCSS` | string | "" | CSS selectors to scope / remove before extraction |
| `MaxTokens` | int | 0 | Approximate output token cap (0 = unlimited) |
| `HeadOnly` | bool | false | Fetch first 16 KB, metadata + canonical only |
| `NoPruneFallback` | bool | false | Disable tag-density fallback for thin output |
| `RespectRobots` | bool | false | Consult robots.txt before fetching |
| `RateLimit` | time.Duration | 0 | Min interval between requests to the same host |
| `UserAgent` | string | "" | Preset name or literal UA string |
| `Proxy` | string | "" | `http(s)://` or `socks5://` proxy URL |
| `CacheDir` | string | "" | Enable on-disk cache at this path |
| `CacheTTL` | time.Duration | — | Cache freshness window |
| `CacheStaleTolerance` | time.Duration | 0 | Stale-while-revalidate window |
| `NoCache` | bool | false | Bypass cache reads |
| `NoPDF` | bool | false | Skip PDF extraction |
| `SchemaPath` | string | "" | CSS schema file for structured extraction |
| `Query` | string | "" | BM25 query to score sections |
| `TopN` | int | 0 | Keep only top-N sections |
| `FilterByQuery` | bool | false | Replace `Content` with top-N sections |
| `SplitOut` / `SplitBytes` | string / int | "" / 0 | Split output across files |
| `Security` | `*SecurityPolicy` | nil | SSRF / private-IP / redirect / decompression guard (see below) |

## Security policy

`Options.Security` threads an SSRF / private-IP / redirect / decompression guard
through the whole fetch path. **A `nil` policy (the zero value) disables every
check** — the historical unguarded behaviour — so a library caller handling
**untrusted URLs must set one.** The CLI and the MCP server apply
`DefaultSecurityPolicy()` automatically.

```go
result := seaportal.FromURLWithOptions(untrustedURL, seaportal.Options{
    Security: seaportal.DefaultSecurityPolicy(), // block private IPs, http/https, caps
})
if result.SecurityBlock != "" {
    // refused by policy (SSRF / scheme / domain / size cap)
}
```

```go
type SecurityPolicy struct {
    BlockPrivateIPs      bool     // reject RFC1918/loopback/link-local/ULA/CGNAT/metadata IPs
    TrustedResolveCIDRs  []string // CIDRs/IPs allowed to resolve non-public (escape hatch)
    AllowedDomains       []string // host allowlist (suffix match); empty = any
    DeniedDomains        []string // host blocklist (suffix match; deny wins)
    AllowedSchemes       []string // default {"http","https"}; rejects file:/ftp:/gopher:/ws:
    MaxRedirects         int      // >0 cap, 0 none, -1 unlimited
    RevalidateRedirects  bool     // re-validate scheme+host+IP on every redirect hop
    MaxResponseBytes     int64    // cap raw body (0 = unbounded)
    MaxDecompressedBytes int64    // cap decompressor output — defuses zip bombs (0 = unbounded)
}
```

`DefaultSecurityPolicy()` returns `BlockPrivateIPs: true`, `http`/`https` only,
`MaxRedirects: 10` with `RevalidateRedirects: true`, and 50 MiB / 200 MiB body
caps. Enforcement runs pre-fetch, at the dial Control hook (closing the
DNS-rebinding window), on every redirect hop, and at the read/decompress
boundary. A refusal sets both `Result.Error` and `Result.SecurityBlock`. See
[SECURITY.md](../../SECURITY.md).

## `Result`

`Result` (`internal/engine/result.go`) marshals to **camelCase JSON**. Core fields:

```go
type Result struct {
    URL          string      `json:"url"`
    CanonicalURL string      `json:"canonicalUrl,omitempty"`
    Title        string      `json:"title"`
    Content      string      `json:"content"`      // extracted Markdown
    Byline       string      `json:"byline"`
    Excerpt      string      `json:"excerpt"`
    SiteName     string      `json:"sitename"`
    Language     string      `json:"language,omitempty"`
    Length       int         `json:"length"`
    TimeMs       int64       `json:"timeMs"`
    Confidence   int         `json:"confidence"`
    IsSPA        bool        `json:"isSpa"`
    IsBlocked    bool        `json:"isBlocked"`
    SPASignals   []string    `json:"spaSignals,omitempty"`
    Quality      float64     `json:"quality"`      // advisory soft signal — do NOT route on it (see note below)
    Profile      PageProfile `json:"profile"`      // classification + browser-routing decision
    PageClass    PageClass   `json:"pageClass"`
    Validation   Validation  `json:"validation"`
    Fingerprint  string      `json:"fingerprint"`
    Error         string     `json:"error,omitempty"`
    SecurityBlock string     `json:"securityBlock,omitempty"` // reason a SecurityPolicy refused the fetch
    StatusCode    int        `json:"statusCode,omitempty"`
    // ... plus cache, timing, redirect, and response-header forensics fields
}
```

> **`quality` is a soft signal, not a gate.** The float is deliberately noisy:
> excellent server-rendered pages routinely score near zero (Wikipedia and GitHub
> score 0; `theguardian.com` extracts ~36k clean chars yet scores 0) while still
> being fully extractable. Route on `profile.decision` / `profile.browserRecommended`
> (and `profile.class` + `profile.outcome`) — never on the raw `quality` float. See
> [browser-discriminator.md](browser-discriminator.md) for why `quality` is not a
> routing input, and [classifier-validation.md](classifier-validation.md) for the
> held-out evidence that the routing decision generalizes (31/31) while the raw
> 6-way class does not (0.71).

## Classification

```go
profile := seaportal.ClassifyPage(result)            // PageProfile
signals, isSPA := seaportal.DetectSPA(htmlString)
blocked := seaportal.DetectBlocked(htmlString)
needsBrowser, reason := seaportal.QuickNeedsBrowser(htmlString)
```

`PageProfile`:

```go
type PageProfile struct {
    Class              PageClass         `json:"class"`
    Outcome            ExtractionOutcome `json:"outcome"`
    Decision           BrowserDecision   `json:"decision"`
    BrowserRecommended bool              `json:"browserRecommended"`
    Reasons            []string          `json:"reasons"`
    Confidence         int               `json:"confidence"`
    Trustworthy        bool              `json:"trustworthy"`
}
```

- `PageClass`: `static`, `ssr`, `hydrated`, `spa`, `dynamic`, `blocked`.
- `ExtractionOutcome`: `extract`, `extract-with-warning`, `fail-fast`, `needs-browser`.
- `BrowserDecision` + `BrowserRecommended` drive browser fall-through — see [browser-discriminator.md](browser-discriminator.md).

## Snapshots

```go
node, err := seaportal.BuildSnapshot(htmlString)
node, err := seaportal.BuildSnapshotWithOptions(htmlString, seaportal.SnapshotOptions{
    FilterInteractive: true,
    MaxTokens:         2000,
})
fmt.Println(node.ToCompact()) // readable text tree
```

`SnapshotNode` fields: `role`, `name`, `tag`, `ref` (e.g. `e5`), `selector`, `depth`, `interactive`, `level`, `value`, `href`, `checked`, `disabled`, `children`.

## Content processing

> Secondary surfaces — opt-in helpers around the core extract primitive, not part of
> the default path. See [Core vs. advanced surfaces](../../README.md#core-vs-advanced-surfaces).

```go
seaportal.Dedupe(content)                            // DedupeResult
seaportal.DedupeWithOptions(content, opts)
seaportal.CleanupMarkdown(md)                         // string
seaportal.PreprocessHTML(html)                        // string
seaportal.RankSections(content, query, k1, b, topN)   // []RankedSection (BM25; k1/b 0 → 1.5/0.75)
seaportal.ChunkMarkdown(md, cfg)                      // []Chunk
seaportal.SplitResultToFiles(result, cfg)             // ([]SplitFile, error)
```

## Sitemaps & feeds

> Secondary surfaces — convenience parsers, not part of the core extract path. See
> [Core vs. advanced surfaces](../../README.md#core-vs-advanced-surfaces).

```go
entries, err := seaportal.FlattenSitemap(ctx, url, seaportal.FlattenSitemapOptions{
    MaxDepth: 5, MaxURLs: 50000, Timeout: 30 * time.Second,
})
items, err := seaportal.ParseFeed(ctx, url, seaportal.ParseFeedOptions{
    MaxItems: 200, Timeout: 30 * time.Second,
})
```

`ParseFeed` handles RSS 2.0, Atom 1.0, and JSON Feed 1.x into a unified `[]FeedItem`.

## Site scraping

`ScrapeSite` runs the whole-site pipeline — discover → group → sample → fetch +
extract + assemble → summarize — and returns a structured `*ScrapeResult`. The
output is designed to be handed to [PinchTab](https://pinchtab.com) for deep
browser enrichment.

```go
result, err := seaportal.ScrapeSite(ctx, &seaportal.ScrapeOptions{
    BaseURL:       "https://example.com",
    MaxPages:      50,
    MaxPerPattern: 8,
    // SampleStrategy, IncludePatterns, ExcludePatterns, Full,
    // WithPerformance, RespectRobots, Timeout, UserAgent …
})
```

`ScrapeOptions` fields map 1:1 to the [`scrape` CLI flags](cli.md#seaportal-scrape).
Zero-valued fields resolve to documented defaults (`MaxPages` 50, `MaxPerPattern`
8, `SampleStrategy` `balanced`, `Output` `json`, `Timeout` 60s, `RespectRobots`
true — note `RespectRobots` is a `*bool` so an unset value defaults to on).
`ScrapeSite` returns `ErrMissingBaseURL` for an empty/invalid base URL.

`ScrapeResult` mirrors the spec output shape:

| Type | Purpose |
|------|---------|
| `ScrapeResult` | `Site`, `PageGroups`, `Pages`, `Summary` |
| `SiteInfo` | `baseURL`, `title`, `discoveredAt`, `sitemapFound`, `totalURLsInSitemap`, `sampledPages` |
| `PageGroup` | one URL-pattern cluster: `pattern`, `totalInSitemap`, `sampled`, `pages` |
| `PageObject` | one page: `url`, `title`, `status`, `meta`, `markdown`, `schema`, `contentType`, `internalLinks`, `externalLinks`, `error` |
| `PagePerformance` | `ttfbMs`, `totalBytes`, `requests` (only when `WithPerformance`) |
| `ScrapeSummary` | `contentTypes` tally + heuristic `recommendations` |

Render helpers turn a result into the three output formats:

```go
data, _ := seaportal.RenderScrapeJSON(result)       // []byte
md := seaportal.RenderScrapeMarkdown(result)         // string
files, _ := seaportal.WriteScrapeDirectory(result, "out/") // result.json + pages/<slug>.md + index.md
```

## Fingerprinting

```go
fp := seaportal.SemanticFingerprint(content)          // string
changed := seaportal.ContentChanged(old, new)         // bool
```

## CLI

The same surface is available from the command line — see [cli.md](cli.md), [mcp.md](mcp.md) (MCP server), and [seabench.md](seabench.md) (benchmark harness).
