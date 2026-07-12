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
	fmt.Fprintln(w, "SeaPortal - Extract clean Markdown from URLs with SPA detection")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  seaportal [options] <url>          Extract Markdown / JSON / snapshot (default verb)")
	fmt.Fprintln(w, "  seaportal sitemap <url> [flags]    Flatten a sitemap.xml (and recurse sitemap-index)")
	fmt.Fprintln(w, "  seaportal feed <url> [flags]       Parse RSS / Atom / JSON Feed into unified entries")
	fmt.Fprintln(w, "  seaportal scrape <url> [flags]     Scrape a whole site into structured output")
	fmt.Fprintln(w, "  seaportal snapshot <url> [flags]   Print the accessibility-tree snapshot for a URL")
	fmt.Fprintln(w, "  seaportal mcp                      Run as an MCP (Model Context Protocol) server over stdio")
	fmt.Fprintln(w, "  seaportal version                  Print the seaportal version")
	fmt.Fprintln(w, "  seaportal help                     Show this help")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run 'seaportal -h' for the full list of extract options.")
}

func main() {
	// One signal-aware context for every verb: Ctrl-C / SIGTERM cancels
	// in-flight fetches instead of leaving them to run to their timeout.
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

// looksLikeBogusVerb reports whether the first CLI arg is a mistyped
// subcommand rather than an extract target: a bare token that is not a flag,
// has no URL scheme, and contains no dot that could make it a host.
func looksLikeBogusVerb(arg string) bool {
	if arg == "" || strings.HasPrefix(arg, "-") {
		return false
	}
	if strings.Contains(arg, ".") || strings.Contains(arg, "/") {
		return false
	}
	if strings.Contains(arg, ":") { // scheme, e.g. https:, data:
		return false
	}
	return true
}

// listFetchTimeout bounds the sitemap / feed fetches.
const listFetchTimeout = 30 * time.Second

// listVerb is the shared scaffold of the sitemap and feed subcommands: one
// URL argument, --json / --allow-internal flags plus verb-specific extras, a
// security-guarded fetch, and line-oriented default output (JSON opt-in).
type listVerb[T any] struct {
	name      string                 // subcommand name; also the error prefix
	usageLine string                 // first line of the -h output
	jsonUsage string                 // --json flag description (differs per verb)
	addFlags  func(fs *flag.FlagSet) // registers verb-specific flags
	fetch     func(ctx context.Context, url string, sec *seaportal.SecurityPolicy) ([]T, error)
	printLine func(T) // default (non-JSON) renderer, one entry per line
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

// splitCSV splits a comma-separated flag value into trimmed, non-empty items.
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

// fetchHTML retrieves url through the shared security-guarded fetch path with
// a browser-style Accept header, bound to ctx so Ctrl-C cancels the fetch.
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
