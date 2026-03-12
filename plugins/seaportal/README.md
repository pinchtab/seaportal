# SeaPortal SMCP Plugin

MCP (Model Context Protocol) plugin for SeaPortal web content extraction.

## Requirements

- Python 3.9+
- `seaportal` CLI in PATH

## Installation

```bash
# Install seaportal
go install github.com/pinchtab/seaportal/cmd/seaportal@latest

# Use plugin with SMCP
smcp add seaportal ./cli.py
```

## Commands

### extract

Extract content from URL as markdown or JSON.

```bash
python cli.py extract --url https://example.com
python cli.py extract --url https://example.com --json
python cli.py extract --url https://example.com --fast
```

### snapshot

Get accessibility tree snapshot from URL.

```bash
python cli.py snapshot --url https://example.com
python cli.py snapshot --url https://example.com --filter interactive
python cli.py snapshot --url https://example.com --format compact
python cli.py snapshot --url https://example.com --max-tokens 2000
```

### version

Get seaportal version.

```bash
python cli.py version
```

## Plugin Description

```bash
python cli.py --describe
```

## Output Format

All commands return JSON:

```json
{
  "status": "success",
  "data": { ... }
}
```

Or on error:

```json
{
  "status": "error",
  "error": "Error message",
  "error_type": "validation_error"
}
```
