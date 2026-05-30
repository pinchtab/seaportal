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
    Quality      float64     `json:"quality"`
    Profile      PageProfile `json:"profile"`      // classification + browser-routing decision
    PageClass    PageClass   `json:"pageClass"`
    Validation   Validation  `json:"validation"`
    Fingerprint  string      `json:"fingerprint"`
    Error        string      `json:"error,omitempty"`
    StatusCode   int         `json:"statusCode,omitempty"`
    // ... plus cache, timing, redirect, and response-header forensics fields
}
```

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

```go
entries, err := seaportal.FlattenSitemap(ctx, url, seaportal.FlattenSitemapOptions{
    MaxDepth: 5, MaxURLs: 50000, Timeout: 30 * time.Second,
})
items, err := seaportal.ParseFeed(ctx, url, seaportal.ParseFeedOptions{
    MaxItems: 200, Timeout: 30 * time.Second,
})
```

`ParseFeed` handles RSS 2.0, Atom 1.0, and JSON Feed 1.x into a unified `[]FeedItem`.

## Fingerprinting

```go
fp := seaportal.SemanticFingerprint(content)          // string
changed := seaportal.ContentChanged(old, new)         // bool
```

## CLI

The same surface is available from the command line — see [cli.md](cli.md), [mcp.md](mcp.md) (MCP server), and [seabench.md](seabench.md) (benchmark harness).
