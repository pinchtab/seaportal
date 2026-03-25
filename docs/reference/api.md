# API Reference

## Package `pkg/portal`

### Functions

#### `Extract(url string, opts *Options) (*Result, error)`

Fetches and extracts content from the given URL.

**Options:**

| Field       | Type   | Default | Description                          |
|-------------|--------|---------|--------------------------------------|
| `Fast`      | bool   | false   | Bail early if browser likely needed  |
| `JSON`      | bool   | false   | Return structured JSON               |
| `Snapshot`  | bool   | false   | Return accessibility tree            |
| `NoDedupe`  | bool   | false   | Disable content deduplication        |

#### `Classify(html []byte) PageType`

Classifies an HTML page into a category.

**Page Types:** `Static`, `SSR`, `Hydrated`, `Dynamic`, `Blocked`

### Types

#### `Result`

```go
type Result struct {
    URL        string   `json:"url"`
    Title      string   `json:"title"`
    Markdown   string   `json:"markdown"`
    PageType   string   `json:"page_type"`
    Quality    float64  `json:"quality"`
    Blocked    bool     `json:"blocked"`
    BlockType  string   `json:"block_type,omitempty"`
}
```

## CLI

See `seaportal --help` for full CLI usage.
