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

## As a Library

```go
package main

import (
    "fmt"
    "github.com/pinchtab/seaportal/pkg/portal"
)

func main() {
    result, err := portal.Extract("https://example.com", nil)
    if err != nil {
        panic(err)
    }
    fmt.Println(result.Markdown)
}
```

## Next Steps

- [Architecture](architecture/design.md) — how SeaPortal works
- [Contributing](guides/contributing.md) — how to contribute
- [API Reference](reference/api.md) — full API docs
