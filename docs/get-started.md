# Getting Started

## Install

### npm (recommended)

```bash
npm install -g seaportal
```

### Go

```bash
go install github.com/pinchtab/seaportal/cmd/seaportal@latest
```

### Binary

Download from [GitHub Releases](https://github.com/pinchtab/seaportal/releases).

## Quick Usage

```bash
# Extract content from a URL
seaportal https://example.com

# JSON output
seaportal --json https://example.com

# Accessibility snapshot
seaportal --snapshot https://example.com

# Fast mode (bail early if browser needed)
seaportal --fast https://example.com
```

### Subcommands

```bash
seaportal sitemap https://example.com/sitemap.xml   # flatten a sitemap
seaportal feed https://example.com/feed.xml         # parse RSS / Atom / JSON Feed
seaportal mcp                                       # run as an MCP server over stdio
```

See the [CLI reference](reference/cli.md) for all flags and the [MCP reference](reference/mcp.md) for the server tools.

## As a Library

The public package is the module root, `github.com/pinchtab/seaportal`:

```go
package main

import (
    "fmt"

    "github.com/pinchtab/seaportal"
)

func main() {
    result := seaportal.FromURL("https://example.com")
    fmt.Println(result.Content) // extracted Markdown
}
```

## Next Steps

- [Architecture](architecture/design.md) — how SeaPortal works
- [Contributing](guides/contributing.md) — how to contribute
- [API Reference](reference/api.md) — Go library API
- [CLI Reference](reference/cli.md) — command-line flags and subcommands
