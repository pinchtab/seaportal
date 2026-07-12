// Package tools registers seaportal's MCP tool surface onto an mcp.Server:
// the five tools that wrap the root library entry points, their JSON schemas,
// argument parsing, and server-side guardrails.
//
//   - fetch_url       → seaportal.FromURLWithOptions
//   - fetch_snapshot  → seaportal.BuildSnapshotWithOptions
//   - parse_sitemap   → seaportal.FlattenSitemap
//   - parse_feed      → seaportal.ParseFeed
//   - scrape_site     → seaportal.ScrapeSite
//
// Tool results are JSON-marshalled and returned as a single text content
// block. The package lives outside cmd/seaportal so alternative transports
// (HTTP/SSE, embedding in PinchTab) can reuse the same tool layer; it imports
// only the root seaportal facade (root → engine, tools → root — no cycle).
package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/pinchtab/seaportal"
	"github.com/pinchtab/seaportal/internal/mcp"
)

// Server-side guardrails for the scrape_site MCP tool.
const (
	maxScrapePages          = 200
	maxScrapePerPattern     = 50
	maxScrapeTimeoutSeconds = 180
)

// Register wires every supported library entry point into srv. Registration
// order is part of the wire contract: tools/list reports tools in this order.
func Register(srv *mcp.Server) {
	registerFetchURL(srv)
	registerFetchSnapshot(srv)
	registerParseSitemap(srv)
	registerParseFeed(srv)
	registerScrapeSite(srv)
}

func registerFetchURL(srv *mcp.Server) {
	srv.RegisterTool(
		"fetch_url",
		"Fetch a URL and return extracted Markdown + metadata as JSON (Result struct).",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":           map[string]interface{}{"type": "string", "description": "URL to fetch"},
				"dedupe":        map[string]interface{}{"type": "boolean", "description": "Enable block dedup (default true)"},
				"fast":          map[string]interface{}{"type": "boolean", "description": "Bail early if browser is needed"},
				"with_links":    map[string]interface{}{"type": "boolean"},
				"with_images":   map[string]interface{}{"type": "boolean"},
				"with_tables":   map[string]interface{}{"type": "boolean"},
				"with_comments": map[string]interface{}{"type": "boolean"},
				"max_tokens":    map[string]interface{}{"type": "integer"},
			},
			"required": []string{"url"},
		},
		func(ctx context.Context, args map[string]interface{}) (string, error) {
			url, err := requiredURLArg(args, "url")
			if err != nil {
				return "", err
			}
			// The MCP server fetches arbitrary URLs from tool args — the most
			// exposed entrypoint — so apply the secure-by-default policy
			// (private-IP block on, http/https only, redirect + body caps).
			opts := seaportal.Options{Dedupe: true, Security: seaportal.DefaultSecurityPolicy()}
			if v, ok := args["dedupe"].(bool); ok {
				opts.Dedupe = v
			}
			if v, ok := args["fast"].(bool); ok {
				opts.FastMode = v
			}
			if v, ok := args["with_links"].(bool); ok {
				opts.WithLinks = v
			}
			if v, ok := args["with_images"].(bool); ok {
				opts.WithImages = v
			}
			if v, ok := args["with_tables"].(bool); ok {
				opts.WithTables = v
			}
			if v, ok := args["with_comments"].(bool); ok {
				opts.WithComments = v
			}
			if v, ok := args["max_tokens"].(float64); ok {
				opts.MaxTokens = int(v)
			}
			// Wire the handler context so a client cancel / server shutdown can
			// interrupt an in-flight retry backoff (ALP-043).
			opts.Context = ctx
			return marshalResult(seaportal.FromURLWithOptions(url, opts), "result")
		},
	)
}

func registerFetchSnapshot(srv *mcp.Server) {
	srv.RegisterTool(
		"fetch_snapshot",
		"Build the accessibility-tree snapshot for a URL (HTTP-only fetch, then snapshot extraction).",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":            map[string]interface{}{"type": "string"},
				"filter":         map[string]interface{}{"type": "string", "description": "'interactive' to keep only links/buttons/inputs; empty for full tree"},
				"max_tokens":     map[string]interface{}{"type": "integer"},
				"allow_internal": map[string]interface{}{"type": "boolean", "description": "Allow private/internal IP targets"},
			},
			"required": []string{"url"},
		},
		func(ctx context.Context, args map[string]interface{}) (string, error) {
			url, err := requiredURLArg(args, "url")
			if err != nil {
				return "", err
			}
			html, err := fetchHTML(ctx, url, securityFromArgs(args))
			if err != nil {
				return "", fmt.Errorf("fetch %s: %w", url, err)
			}
			opts := seaportal.SnapshotOptions{}
			if v, ok := args["filter"].(string); ok {
				opts.FilterInteractive = v == "interactive"
			}
			if v, ok := args["max_tokens"].(float64); ok {
				opts.MaxTokens = int(v)
			}
			tree, err := seaportal.BuildSnapshotWithOptions(html, opts)
			if err != nil {
				return "", fmt.Errorf("build snapshot: %w", err)
			}
			return marshalResult(tree, "snapshot")
		},
	)
}

func registerParseSitemap(srv *mcp.Server) {
	srv.RegisterTool(
		"parse_sitemap",
		"Fetch and flatten a sitemap.xml (nested <sitemapindex> supported, .gz auto-decompressed). Returns JSON array of {loc,lastmod,changefreq,priority}.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":            map[string]interface{}{"type": "string"},
				"max_depth":      map[string]interface{}{"type": "integer", "description": "Max sitemap-index recursion depth (default 5)"},
				"max_urls":       map[string]interface{}{"type": "integer", "description": "Stop after this many URLs (default 50000)"},
				"allow_internal": map[string]interface{}{"type": "boolean", "description": "Allow private/internal IP targets"},
			},
			"required": []string{"url"},
		},
		func(ctx context.Context, args map[string]interface{}) (string, error) {
			url, err := requiredURLArg(args, "url")
			if err != nil {
				return "", err
			}
			opts := seaportal.FlattenSitemapOptions{
				MaxDepth: 5,
				MaxURLs:  50000,
				Timeout:  30 * time.Second,
				Security: securityFromArgs(args),
			}
			if v, ok := args["max_depth"].(float64); ok {
				opts.MaxDepth = int(v)
			}
			if v, ok := args["max_urls"].(float64); ok {
				opts.MaxURLs = int(v)
			}
			entries, err := seaportal.FlattenSitemap(ctx, url, opts)
			if err != nil {
				return "", fmt.Errorf("flatten sitemap: %w", err)
			}
			return marshalResult(entries, "entries")
		},
	)
}

func registerParseFeed(srv *mcp.Server) {
	srv.RegisterTool(
		"parse_feed",
		"Fetch and parse an RSS 2.0, Atom 1.0 or JSON Feed 1.x URL into a unified {title,link,published,summary,author,guid} JSON array.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":            map[string]interface{}{"type": "string"},
				"max_items":      map[string]interface{}{"type": "integer", "description": "Stop after this many items (default 200)"},
				"allow_internal": map[string]interface{}{"type": "boolean", "description": "Allow private/internal IP targets"},
			},
			"required": []string{"url"},
		},
		func(ctx context.Context, args map[string]interface{}) (string, error) {
			url, err := requiredURLArg(args, "url")
			if err != nil {
				return "", err
			}
			opts := seaportal.ParseFeedOptions{
				MaxItems: 200,
				Timeout:  30 * time.Second,
				Security: securityFromArgs(args),
			}
			if v, ok := args["max_items"].(float64); ok {
				opts.MaxItems = int(v)
			}
			items, err := seaportal.ParseFeed(ctx, url, opts)
			if err != nil {
				return "", fmt.Errorf("parse feed: %w", err)
			}
			return marshalResult(items, "items")
		},
	)
}

func registerScrapeSite(srv *mcp.Server) {
	srv.RegisterTool(
		"scrape_site",
		"Scrape a whole site from a base URL: discover (robots/sitemap/crawl), group similar URLs, sample, fetch+extract each page, and return a structured ScrapeResult JSON (site, pageGroups, pages, summary) designed for PinchTab enrichment.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"base_url":         map[string]interface{}{"type": "string", "description": "Base URL to scrape"},
				"max_pages":        map[string]interface{}{"type": "integer", "description": "Max total pages (default 50, capped at 200)"},
				"max_per_pattern":  map[string]interface{}{"type": "integer", "description": "Max samples per URL pattern (default 8, capped at 50)"},
				"full":             map[string]interface{}{"type": "boolean", "description": "Disable sampling (fetch all discovered pages, still capped)"},
				"include_patterns": map[string]interface{}{"type": "string", "description": "Comma-separated glob patterns to include"},
				"exclude_patterns": map[string]interface{}{"type": "string", "description": "Comma-separated glob patterns to exclude"},
				"sample_strategy":  map[string]interface{}{"type": "string", "description": "random|priority|balanced (default balanced)"},
				"with_performance": map[string]interface{}{"type": "boolean", "description": "Include per-page performance data"},
				"respect_robots":   map[string]interface{}{"type": "boolean", "description": "Respect robots.txt disallow + crawl-delay (default true)"},
				"timeout_seconds":  map[string]interface{}{"type": "integer", "description": "Overall timeout in seconds (default 60, capped at 180)"},
				"user_agent":       map[string]interface{}{"type": "string"},
			},
			"required": []string{"base_url"},
		},
		func(ctx context.Context, args map[string]interface{}) (string, error) {
			baseURL, err := requiredURLArg(args, "base_url")
			if err != nil {
				return "", err
			}

			strategy := seaportal.SampleBalanced
			if v, ok := args["sample_strategy"].(string); ok && v != "" {
				strategy = seaportal.SampleStrategy(v)
				switch strategy {
				case seaportal.SampleBalanced, seaportal.SampleRandom, seaportal.SamplePriority:
				default:
					return "", fmt.Errorf("unknown sample_strategy %q (want balanced|random|priority)", v)
				}
			}

			// Server-side guardrails: cap page budget and timeout so a single
			// MCP call can't fan out unboundedly.
			maxPages := argInt(args, "max_pages", 50)
			if maxPages > maxScrapePages {
				maxPages = maxScrapePages
			}
			maxPerPattern := argInt(args, "max_per_pattern", 8)
			if maxPerPattern > maxScrapePerPattern {
				maxPerPattern = maxScrapePerPattern
			}
			timeoutSec := argInt(args, "timeout_seconds", 60)
			if timeoutSec <= 0 || timeoutSec > maxScrapeTimeoutSeconds {
				timeoutSec = maxScrapeTimeoutSeconds
			}

			respectRobots := true
			if v, ok := args["respect_robots"].(bool); ok {
				respectRobots = v
			}
			opts := &seaportal.ScrapeOptions{
				BaseURL:         baseURL,
				MaxPages:        maxPages,
				MaxPerPattern:   maxPerPattern,
				SampleStrategy:  strategy,
				IncludePatterns: splitCSV(argString(args, "include_patterns")),
				ExcludePatterns: splitCSV(argString(args, "exclude_patterns")),
				RespectRobots:   &respectRobots,
				Timeout:         time.Duration(timeoutSec) * time.Second,
				UserAgent:       argString(args, "user_agent"),
			}
			if v, ok := args["full"].(bool); ok {
				opts.Full = v
			}
			if v, ok := args["with_performance"].(bool); ok {
				opts.WithPerformance = v
			}

			res, err := seaportal.ScrapeSite(ctx, opts)
			if err != nil {
				return "", fmt.Errorf("scrape site: %w", err)
			}
			return marshalResult(res, "result")
		},
	)
}
