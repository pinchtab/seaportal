package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pinchtab/seaportal"
)

var version = "dev"

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "SeaPortal - Extract clean Markdown from URLs with SPA detection")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  seaportal [options] <url>          Extract Markdown / JSON / snapshot (default verb)")
	_, _ = fmt.Fprintln(w, "  seaportal sitemap <url> [flags]    Flatten a sitemap.xml (and recurse sitemap-index)")
	_, _ = fmt.Fprintln(w, "  seaportal feed <url> [flags]       Parse RSS / Atom / JSON Feed into unified entries")
	_, _ = fmt.Fprintln(w, "  seaportal scrape <url> [flags]     Scrape a whole site into structured output")
	_, _ = fmt.Fprintln(w, "  seaportal snapshot <url> [flags]   Print the accessibility-tree snapshot for a URL")
	_, _ = fmt.Fprintln(w, "  seaportal mcp                      Run as an MCP (Model Context Protocol) server over stdio")
	_, _ = fmt.Fprintln(w, "  seaportal version                  Print the seaportal version")
	_, _ = fmt.Fprintln(w, "  seaportal help                     Show this help")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Run 'seaportal -h' for the full list of extract options.")
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "sitemap":
			runSitemap(ctx, os.Args[2:])
			return
		case "feed":
			runFeed(ctx, os.Args[2:])
			return
		case "scrape":
			runScrape(ctx, os.Args[2:])
			return
		case "snapshot":
			runSnapshot(ctx, os.Args[2:])
			return
		case "mcp":
			runMCP(ctx, os.Args[2:])
			return
		case "help":
			printUsage(os.Stdout)
			return
		case "version":
			fmt.Printf("seaportal %s\n", version)
			return
		}
		if arg := os.Args[1]; looksLikeBogusVerb(arg) {
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", arg)
			fmt.Fprintln(os.Stderr, "run 'seaportal help' for usage")
			os.Exit(2)
		}
	}
	runExtract(ctx, os.Args[1:])
}

func looksLikeBogusVerb(arg string) bool {
	if arg == "" || strings.HasPrefix(arg, "-") {
		return false
	}
	if strings.Contains(arg, ".") || strings.Contains(arg, "/") {
		return false
	}
	if strings.Contains(arg, ":") {
		return false
	}
	return true
}

const listFetchTimeout = 30 * time.Second

type listVerb[T any] struct {
	name      string
	usageLine string
	jsonUsage string
	addFlags  func(fs *flag.FlagSet)
	fetch     func(ctx context.Context, url string, sec *seaportal.SecurityPolicy) ([]T, error)
	printLine func(T)
}

func (v listVerb[T]) run(ctx context.Context, args []string) {
	fs := flag.NewFlagSet(v.name, flag.ExitOnError)
	jsonOut := fs.Bool("json", false, v.jsonUsage)
	allowInternal := fs.Bool("allow-internal", false, "Allow private/internal IP targets")
	v.addFlags(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, v.usageLine)
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
	entries, err := v.fetch(ctx, fs.Arg(0), sec)
	if err != nil {
		fmt.Fprintln(os.Stderr, v.name+" error:", err)
		os.Exit(1)
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		if err := enc.Encode(entries); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}
	for _, e := range entries {
		v.printLine(e)
	}
}

func runSitemap(ctx context.Context, args []string) {
	var maxURLs, maxDepth *int
	listVerb[seaportal.SitemapEntry]{
		name:      "sitemap",
		usageLine: "Usage: seaportal sitemap <url> [--json] [--max-urls N] [--max-depth N]",
		jsonUsage: "Emit JSON array instead of newline-separated URLs",
		addFlags: func(fs *flag.FlagSet) {
			maxURLs = fs.Int("max-urls", seaportal.DefaultSitemapMaxURLs, "Stop after this many URLs")
			maxDepth = fs.Int("max-depth", seaportal.DefaultSitemapMaxDepth, "Max sitemap-index recursion depth")
		},
		fetch: func(ctx context.Context, url string, sec *seaportal.SecurityPolicy) ([]seaportal.SitemapEntry, error) {
			return seaportal.FlattenSitemap(ctx, url, seaportal.FlattenSitemapOptions{
				MaxDepth: *maxDepth,
				MaxURLs:  *maxURLs,
				Timeout:  listFetchTimeout,
				Security: sec,
			})
		},
		printLine: func(e seaportal.SitemapEntry) { fmt.Println(e.Loc) },
	}.run(ctx, args)
}

func runFeed(ctx context.Context, args []string) {
	var maxItems *int
	listVerb[seaportal.FeedItem]{
		name:      "feed",
		usageLine: "Usage: seaportal feed <url> [--json] [--max-items N]",
		jsonUsage: "Emit JSON array instead of TSV lines",
		addFlags: func(fs *flag.FlagSet) {
			maxItems = fs.Int("max-items", seaportal.DefaultFeedMaxItems, "Stop after this many items")
		},
		fetch: func(ctx context.Context, url string, sec *seaportal.SecurityPolicy) ([]seaportal.FeedItem, error) {
			return seaportal.ParseFeed(ctx, url, seaportal.ParseFeedOptions{
				MaxItems: *maxItems,
				Timeout:  listFetchTimeout,
				Security: sec,
			})
		},
		printLine: func(e seaportal.FeedItem) { fmt.Printf("%s\t%s\t%s\n", e.Published, e.Title, e.Link) },
	}.run(ctx, args)
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func fetchHTML(ctx context.Context, url string, sec *seaportal.SecurityPolicy) (string, error) {
	body, _, _, err := seaportal.FetchBytes(ctx, url, seaportal.FetchBytesOptions{
		Timeout:  30 * time.Second,
		Security: sec,
		Accept:   "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	})
	if err != nil {
		return "", err
	}
	return string(body), nil
}
