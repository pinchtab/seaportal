package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pinchtab/seaportal"
	"github.com/pinchtab/seaportal/internal/mcp"
)

// runMCP starts seaportal as an MCP (Model Context Protocol) server over
// JSON-RPC 2.0 line-delimited stdio. It exposes four tools that wrap the
// existing library entry points:
//
//   - fetch_url       → seaportal.FromURLWithOptions
//   - fetch_snapshot  → seaportal.BuildSnapshotWithOptions
//   - parse_sitemap   → seaportal.FlattenSitemap
//   - parse_feed      → seaportal.ParseFeed
//
// Tool results are JSON-marshalled and returned as a single text content
// block. No flags are accepted: configuration flows through MCP tool arguments.
func runMCP(_ []string) {
	srv := mcp.NewServer()
	srv.SetIdentity("seaportal", version)
	registerMCPTools(srv)

	if err := srv.ServeStdio(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "mcp server error:", err)
		os.Exit(1)
	}
}

// registerMCPTools wires every supported library entry point into srv.
// Kept separate from runMCP so tests can drive the same tool surface.
func registerMCPTools(srv *mcp.Server) {
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
		func(args map[string]interface{}) (string, error) {
			url, _ := args["url"].(string)
			if url == "" {
				return "", fmt.Errorf("missing required argument: url")
			}
			opts := seaportal.Options{Dedupe: true}
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
			result := seaportal.FromURLWithOptions(url, opts)
			b, err := json.Marshal(result)
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(b), nil
		},
	)

	srv.RegisterTool(
		"fetch_snapshot",
		"Build the accessibility-tree snapshot for a URL (HTTP-only fetch, then snapshot extraction).",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":        map[string]interface{}{"type": "string"},
				"filter":     map[string]interface{}{"type": "string", "description": "'interactive' to keep only links/buttons/inputs; empty for full tree"},
				"max_tokens": map[string]interface{}{"type": "integer"},
			},
			"required": []string{"url"},
		},
		func(args map[string]interface{}) (string, error) {
			url, _ := args["url"].(string)
			if url == "" {
				return "", fmt.Errorf("missing required argument: url")
			}
			html, err := fetchHTML(url)
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
			b, err := json.Marshal(tree)
			if err != nil {
				return "", fmt.Errorf("marshal snapshot: %w", err)
			}
			return string(b), nil
		},
	)

	srv.RegisterTool(
		"parse_sitemap",
		"Fetch and flatten a sitemap.xml (nested <sitemapindex> supported, .gz auto-decompressed). Returns JSON array of {loc,lastmod,changefreq,priority}.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":       map[string]interface{}{"type": "string"},
				"max_depth": map[string]interface{}{"type": "integer", "description": "Max sitemap-index recursion depth (default 5)"},
				"max_urls":  map[string]interface{}{"type": "integer", "description": "Stop after this many URLs (default 50000)"},
			},
			"required": []string{"url"},
		},
		func(args map[string]interface{}) (string, error) {
			url, _ := args["url"].(string)
			if url == "" {
				return "", fmt.Errorf("missing required argument: url")
			}
			opts := seaportal.FlattenSitemapOptions{
				MaxDepth: 5,
				MaxURLs:  50000,
				Timeout:  30 * time.Second,
			}
			if v, ok := args["max_depth"].(float64); ok {
				opts.MaxDepth = int(v)
			}
			if v, ok := args["max_urls"].(float64); ok {
				opts.MaxURLs = int(v)
			}
			entries, err := seaportal.FlattenSitemap(context.Background(), url, opts)
			if err != nil {
				return "", fmt.Errorf("flatten sitemap: %w", err)
			}
			b, err := json.Marshal(entries)
			if err != nil {
				return "", fmt.Errorf("marshal entries: %w", err)
			}
			return string(b), nil
		},
	)

	srv.RegisterTool(
		"parse_feed",
		"Fetch and parse an RSS 2.0, Atom 1.0 or JSON Feed 1.x URL into a unified {title,link,published,summary,author,guid} JSON array.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":       map[string]interface{}{"type": "string"},
				"max_items": map[string]interface{}{"type": "integer", "description": "Stop after this many items (default 200)"},
			},
			"required": []string{"url"},
		},
		func(args map[string]interface{}) (string, error) {
			url, _ := args["url"].(string)
			if url == "" {
				return "", fmt.Errorf("missing required argument: url")
			}
			opts := seaportal.ParseFeedOptions{
				MaxItems: 200,
				Timeout:  30 * time.Second,
			}
			if v, ok := args["max_items"].(float64); ok {
				opts.MaxItems = int(v)
			}
			items, err := seaportal.ParseFeed(context.Background(), url, opts)
			if err != nil {
				return "", fmt.Errorf("parse feed: %w", err)
			}
			b, err := json.Marshal(items)
			if err != nil {
				return "", fmt.Errorf("marshal items: %w", err)
			}
			return string(b), nil
		},
	)
}
