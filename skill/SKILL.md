---
name: seaportal
description: Extract clean markdown and accessibility snapshots from URLs for AI agents. Detects SPAs, deduplicates content, and outputs element refs for browser automation.
metadata:
  short-description: Web content extraction and accessibility snapshots
---

# SeaPortal

Fast, lightweight content extraction for AI agents — markdown + accessibility tree.

## Quick Start

```bash
# Install
go install github.com/pinchtab/seaportal/cmd/seaportal@latest

# Extract content as markdown
seaportal https://example.com

# Get accessibility tree (for automation)
seaportal --snapshot --filter=interactive https://example.com
```

## Setup

```bash
# Go install (recommended)
go install github.com/pinchtab/seaportal/cmd/seaportal@latest

# From source
git clone https://github.com/pinchtab/seaportal
cd seaportal
go build -o seaportal ./cmd/seaportal
```

## Core Workflow

1. **Extract content**: `seaportal <url>` for readable markdown
2. **Analyze structure**: `seaportal --snapshot <url>` for accessibility tree
3. **Filter interactive**: `--filter=interactive` for clickable elements with refs
4. Use refs (e1, e2) with browser automation tools

## Content Extraction

```bash
seaportal <url>                    # Extract as markdown (default)
seaportal --json <url>             # Extract as JSON with metadata
seaportal --fast <url>             # Bail early if browser needed
seaportal --no-dedupe <url>        # Disable duplicate paragraph removal
```

## Accessibility Snapshot

```bash
seaportal --snapshot <url>                      # Full accessibility tree (JSON)
seaportal --snapshot --filter=interactive <url> # Interactive elements only (recommended)
seaportal --snapshot --format=compact <url>     # Compact text output
seaportal --snapshot --max-tokens=2000 <url>    # Limit output size
```

## Combine Options

```bash
seaportal --snapshot --filter=interactive --format=compact <url>
seaportal --snapshot --filter=interactive --max-tokens=1000 <url>
```

## What a Snapshot Looks Like

### JSON format (default)

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
        {
          "role": "link",
          "name": "Home",
          "tag": "a",
          "ref": "e2",
          "selector": "a.nav-link",
          "depth": 1,
          "href": "/",
          "interactive": true
        }
      ]
    }
  ]
}
```

### Compact format (`--format=compact`)

```
document
  e1 navigation "Main" <nav>
    e2 link "Home" <a> [interactive] href=/
    e3 link "Docs" <a> [interactive] href=/docs
  e4 main <main>
    e5 heading "Welcome" <h1> level=1
    e6 button "Get Started" <button> [interactive]
```

## Snapshot Fields

| Field | Description |
|-------|-------------|
| `role` | Accessibility role (heading, link, button, textbox, etc.) |
| `name` | Accessible name (from aria-label, title, alt, or text) |
| `tag` | HTML tag name (div, a, button, etc.) |
| `ref` | Element reference (e1, e2...) for targeting |
| `selector` | CSS selector for the element |
| `depth` | Nesting depth in the tree |
| `interactive` | Whether the element can be clicked/typed |
| `level` | Heading level (1-6) for headings |
| `href` | Link target for links |
| `value` | Current value for inputs |
| `checked` | Checkbox/radio state |
| `disabled` | Whether element is disabled |

## Content Extraction Output

### Markdown (default)

Includes YAML frontmatter with metadata:

```yaml
---
title: "Page Title"
url: https://example.com
byline: "Author Name"
excerpt: "Page description..."
sitename: "Example Site"
length: 5432
confidence: 85
isSpa: false
pageClass: article
outcome: success
trustworthy: true
---

# Page Title

Content extracted as clean markdown...
```

### JSON (`--json`)

Full metadata including SPA detection, validation, and classification:

```json
{
  "url": "https://example.com",
  "title": "Page Title",
  "content": "# Markdown content...",
  "confidence": 85,
  "isSPA": false,
  "spaSignals": [],
  "profile": {
    "class": "article",
    "outcome": "success",
    "trustworthy": true
  },
  "validation": {
    "isValid": true,
    "needsBrowser": false,
    "confidence": 0.95
  }
}
```

## Example: Analyze Login Page

```bash
seaportal --snapshot --filter=interactive https://example.com/login
```

Output shows interactive elements with refs:
```
e1 textbox "Email" <input> [interactive]
e2 textbox "Password" <input> [interactive]
e3 button "Submit" <button> [interactive]
```

## Example: Extract Article

```bash
seaportal https://blog.example.com/article
# Saves markdown to renders/seaportal/<domain>_<timestamp>.md
```

## Example: Check if Page Needs Browser

```bash
seaportal --fast https://spa-app.com
# Bails early if SPA detected, avoiding unnecessary processing
```

## Options

| Option | Description |
|--------|-------------|
| `--snapshot` | Output accessibility tree instead of content |
| `--filter=interactive` | Show only interactive elements |
| `--format=compact` | Compact text output (vs JSON) |
| `--max-tokens=N` | Approximate token limit for output |
| `--json` | JSON output for content extraction |
| `--fast` | Bail early if browser would be needed |
| `--no-dedupe` | Disable duplicate paragraph removal |
| `-v, --version` | Show version |

## Tips

- Use `--filter=interactive` to reduce noise and focus on actionable elements
- Use `--format=compact` for smaller context in AI prompts
- Use `--max-tokens` to fit output within token limits
- Refs (e1, e2...) are stable within a single snapshot
- Re-run snapshot after page navigation or DOM changes
- Selectors prefer `#id` > `.class` > `tag:nth-of-type(n)`
- Depth 0 elements are top-level; nested elements have depth > 0

## Troubleshooting

- If content is empty, the page may be an SPA — check `isSPA` in JSON output
- If `needsBrowser: true`, the page requires JavaScript rendering
- Use `--fast` to quickly detect SPAs without full processing

## Reporting Issues

Open an issue at https://github.com/pinchtab/seaportal
