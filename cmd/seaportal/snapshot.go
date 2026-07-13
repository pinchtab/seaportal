package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/pinchtab/seaportal"
)

func runSnapshot(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	filter := fs.String("filter", "", "Snapshot filter: 'interactive' to show only interactive elements")
	format := fs.String("format", "json", "Snapshot format: 'json' or 'compact'")
	maxTokens := fs.Int("max-tokens", 0, "Approximate token limit for the snapshot tree (0 = unlimited)")
	allowInternal := fs.Bool("allow-internal", false, "Allow private/internal IP targets")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: seaportal snapshot <url> [--filter interactive] [--format json|compact] [--max-tokens N]")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() == 0 {
		fs.Usage()
		os.Exit(2)
	}

	sec := seaportal.DefaultSecurityPolicy()
	if *allowInternal {
		sec.BlockPrivateIPs = false
	}
	htmlContent, err := fetchHTML(ctx, fs.Arg(0), sec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching URL: %v\n", err)
		os.Exit(1)
	}
	if err := renderSnapshot(os.Stdout, htmlContent, *filter, *format, *maxTokens); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func renderSnapshot(w io.Writer, htmlContent, filter, format string, maxTokens int) error {
	tree, err := seaportal.BuildSnapshotWithOptions(htmlContent, seaportal.SnapshotOptions{
		FilterInteractive: filter == "interactive",
		MaxTokens:         maxTokens,
	})
	if err != nil {
		return fmt.Errorf("building snapshot: %w", err)
	}
	if format == "compact" {
		_, _ = fmt.Fprintln(w, tree.ToCompact())
		return nil
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(tree); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}
