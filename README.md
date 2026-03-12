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
seaportal --fast https://pinchtab.com       # Bail early if browser needed
seaportal --no-dedupe https://pinchtab.com  # Disable deduplication

# Version
seaportal --version
```

## As a Library

```go
import "github.com/pinchtab/seaportal/pkg/portal"

// Simple extraction
result := portal.FromURL("https://pinchtab.com")

// With options
result := portal.FromURLWithOptions("https://pinchtab.com", portal.Options{
    Dedupe:   true,
    FastMode: true,
})

// From HTML string
result := portal.FromHTML(htmlString, "https://pinchtab.com")
```

## Result

```go
type Result struct {
    URL        string
    Title      string
    Content    string   // Markdown
    Length     int
    Confidence int      // 0-100
    
    // Classification
    IsSPA      bool
    IsBlocked  bool
    Profile    PageProfile  // static, ssr, hydrated, spa, dynamic, blocked
    
    // Validation
    Validation ValidationResult
}
```

Use `result.Validation.NeedsBrowser` to decide if browser fallback is required.

## Features

- **Fast** — Pure HTTP, typically <2s per extraction
- **Stealthy** — Chrome TLS fingerprint, realistic headers
- **Smart** — Readability extraction + Markdown conversion
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
