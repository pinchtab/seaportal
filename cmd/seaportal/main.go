package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pinchtab/seaportal"
)

var version = "dev"

func printUsage() {
	fmt.Fprintln(os.Stderr, "SeaPortal - Extract clean Markdown from URLs with SPA detection")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  seaportal [options] <url>          Extract Markdown / JSON / snapshot (default verb)")
	fmt.Fprintln(os.Stderr, "  seaportal sitemap <url> [flags]    Flatten a sitemap.xml (and recurse sitemap-index)")
	fmt.Fprintln(os.Stderr, "  seaportal feed <url> [flags]       Parse RSS / Atom / JSON Feed into unified entries")
	fmt.Fprintln(os.Stderr, "  seaportal mcp                      Run as an MCP (Model Context Protocol) server over stdio")
	fmt.Fprintln(os.Stderr, "  seaportal help                     Show this help")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'seaportal -h' for the full list of extract options.")
}

func runSitemap(args []string) {
	fs := flag.NewFlagSet("sitemap", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Emit JSON array instead of newline-separated URLs")
	maxURLs := fs.Int("max-urls", 50000, "Stop after this many URLs")
	maxDepth := fs.Int("max-depth", 5, "Max sitemap-index recursion depth")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: seaportal sitemap <url> [--json] [--max-urls N] [--max-depth N]")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() == 0 {
		fs.Usage()
		os.Exit(2)
	}
	sitemapURL := fs.Arg(0)
	ctx := context.Background()
	entries, err := seaportal.FlattenSitemap(ctx, sitemapURL, seaportal.FlattenSitemapOptions{
		MaxDepth: *maxDepth,
		MaxURLs:  *maxURLs,
		Timeout:  30 * time.Second,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "sitemap error:", err)
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
		fmt.Println(e.Loc)
	}
}

func runFeed(args []string) {
	fs := flag.NewFlagSet("feed", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Emit JSON array instead of TSV lines")
	maxItems := fs.Int("max-items", 200, "Stop after this many items")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: seaportal feed <url> [--json] [--max-items N]")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() == 0 {
		fs.Usage()
		os.Exit(2)
	}
	feedURL := fs.Arg(0)
	ctx := context.Background()
	entries, err := seaportal.ParseFeed(ctx, feedURL, seaportal.ParseFeedOptions{
		MaxItems: *maxItems,
		Timeout:  30 * time.Second,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "feed error:", err)
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
		fmt.Printf("%s\t%s\t%s\n", e.Published, e.Title, e.Link)
	}
}

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "sitemap":
			runSitemap(os.Args[2:])
			return
		case "feed":
			runFeed(os.Args[2:])
			return
		case "mcp":
			runMCP(os.Args[2:])
			return
		case "help":
			printUsage()
			return
		}
	}
	runExtract(os.Args[1:])
}

func runExtract(rawArgs []string) {
	// Use a fresh FlagSet bound as the package CommandLine so the existing
	// flag.* helper calls below continue to work unchanged. We parse rawArgs
	// rather than os.Args directly to support subcommand dispatch.
	cli := flag.NewFlagSet("seaportal", flag.ExitOnError)
	flag.CommandLine = cli
	noDedupe := flag.Bool("no-dedupe", false, "Disable deduplication (enabled by default)")
	noNearDedupe := flag.Bool("no-near-dedupe", false, "Disable simhash near-duplicate detection (enabled by default)")
	fast := flag.Bool("fast", false, "Fast mode: bail early if browser is needed")
	probeSearch := flag.Bool("probe-search", false, "Override outcome to needs-browser when a search URL returns no result-list structure")
	jsonOut := flag.Bool("json", false, "Output JSON instead of Markdown")
	xmlOut := flag.Bool("xml", false, "Output TEI-Lite XML instead of Markdown")
	snapshot := flag.Bool("snapshot", false, "Output accessibility tree as JSON")
	snapshotFilter := flag.String("filter", "", "Snapshot filter: 'interactive' to show only interactive elements")
	snapshotFormat := flag.String("format", "json", "Snapshot format: 'json' or 'compact'")
	maxTokens := flag.Int("max-tokens", 0, "Approximate token limit for output (snapshot tree OR Markdown body; 0 = unlimited)")
	retries := flag.Int("retries", 3, "Max retry attempts for transient failures (502/503/504/429)")
	maxRetryWait := flag.Duration("max-retry-wait", 30*time.Second, "Max single backoff wait")
	retryTimeout := flag.Duration("retry-timeout", 90*time.Second, "Total budget for all retries")
	withLinks := flag.Bool("with-links", false, "Emit list of discovered <a> links with text + rel")
	withImages := flag.Bool("with-images", false, "Emit list of discovered <img> entries with src/alt/srcset")
	withTables := flag.Bool("with-tables", false, "Emit structured tables (caption/headers/rows) in result")
	withComments := flag.Bool("with-comments", false, "Emit user-generated comments separately in Result.Comments")
	citations := flag.Bool("citations", false, "Convert inline links to numbered references with a References section at the bottom (synonym for --links=footer)")
	linksMode := flag.String("links", "all", "Markdown link retention: none|text|all|footer")
	chunk := flag.String("chunk", "", "Chunking strategy: heading | sentence[:SIZE] | window[:SIZE[:OVERLAP]]")
	selectCSS := flag.String("select", "", "CSS selector(s) to scope extraction (comma-separated)")
	stripCSS := flag.String("strip", "", "CSS selector(s) to remove before extraction (comma-separated)")
	headOnly := flag.Bool("head-only", false, "Fetch only the first 16 KB and extract metadata + canonical (no body extraction)")
	noPruneFallback := flag.Bool("no-prune-fallback", false, "Disable the tag-density heuristic fallback when readability output looks thin")
	respectRobots := flag.Bool("respect-robots", false, "Consult robots.txt and refuse to fetch disallowed paths")
	rateLimit := flag.Duration("rate-limit", 0, "Minimum interval between requests to the same host (e.g. 500ms, 2s)")
	ua := flag.String("ua", "", "User-Agent: preset name (chrome|safari|firefox|googlebot|bingbot|seaportal|search-bot) or literal UA string")
	baseURL := flag.String("base-url", "", "Base URL for stdin HTML input (used to resolve relative links and host-aware checks)")
	proxy := flag.String("proxy", "", "Proxy URL: http://user:pass@host:port, https://..., or socks5://...")
	cacheDir := flag.String("cache", "", "Enable on-disk cache (give a directory path)")
	cacheTTL := flag.Duration("cache-ttl", 24*time.Hour, "Cache freshness window (e.g. 1h, 24h)")
	cacheStaleTolerance := flag.Duration("cache-stale-tolerance", 0, "SWR: serve cached entries stale (background revalidate) within TTL+tolerance window")
	noCache := flag.Bool("no-cache", false, "Bypass cache reads (writes still happen if --cache is set)")
	noPDF := flag.Bool("no-pdf", false, "Skip PDF extraction (treat application/pdf as binary, original pre-PDF behaviour)")
	schemaPath := flag.String("schema", "", "Path to a CSS schema (JSON/YAML) to extract structured data into result.schema")
	query := flag.String("query", "", "Score sections by BM25 relevance to this query")
	topN := flag.Int("top-n", 0, "Keep only the top-N most relevant sections (0 = all)")
	filterByQuery := flag.Bool("filter-by-query", false, "Replace Content with concatenated top-N sections (default top-3 when --top-n is unset)")
	splitOut := flag.String("split-out", "", "Directory to write split output files into")
	splitBytes := flag.Int("split-bytes", 0, "Approximate bytes per split file (default: --max-tokens × 4 or 32768)")
	showVersion := flag.Bool("version", false, "Show version")
	flag.BoolVar(showVersion, "v", false, "Show version")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "SeaPortal - Extract clean Markdown from URLs with SPA detection")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  seaportal [options] <url>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}

	_ = cli.Parse(rawArgs)

	if *showVersion {
		fmt.Printf("seaportal %s\n", version)
		return
	}

	if *xmlOut && *jsonOut {
		fmt.Fprintln(os.Stderr, "error: --xml and --json are mutually exclusive")
		os.Exit(2)
	}

	if *splitOut != "" && *xmlOut {
		fmt.Fprintln(os.Stderr, "error: --split-out is not supported with --xml")
		os.Exit(2)
	}

	args := flag.Args()
	stdinMode := len(args) == 0 || (len(args) == 1 && args[0] == "-")

	var targetURL string
	var stdinHTML string
	if stdinMode {
		if *baseURL == "" {
			fmt.Fprintln(os.Stderr, "error: --base-url is required when reading HTML from stdin")
			os.Exit(2)
		}
		htmlBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: failed to read stdin: %v\n", err)
			os.Exit(2)
		}
		if len(htmlBytes) == 0 {
			fmt.Fprintln(os.Stderr, "error: no HTML provided on stdin")
			os.Exit(2)
		}
		stdinHTML = string(htmlBytes)
		targetURL = *baseURL

		if *headOnly {
			fmt.Fprintln(os.Stderr, "warning: --head-only ignored in stdin mode")
			*headOnly = false
		}
		if *respectRobots {
			fmt.Fprintln(os.Stderr, "warning: --respect-robots ignored in stdin mode")
			*respectRobots = false
		}
		if *retries != 3 {
			fmt.Fprintln(os.Stderr, "warning: --retries ignored in stdin mode")
		}
	} else {
		if len(args) < 1 {
			flag.Usage()
			os.Exit(1)
		}
		targetURL = args[0]
	}

	if *snapshot {
		var htmlContent string
		if stdinMode {
			htmlContent = stdinHTML
		} else {
			h, err := fetchHTML(targetURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error fetching URL: %v\n", err)
				os.Exit(1)
			}
			htmlContent = h
		}

		opts := seaportal.SnapshotOptions{
			FilterInteractive: *snapshotFilter == "interactive",
			MaxTokens:         *maxTokens,
		}

		tree, err := seaportal.BuildSnapshotWithOptions(htmlContent, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error building snapshot: %v\n", err)
			os.Exit(1)
		}

		if *snapshotFormat == "compact" {
			fmt.Println(tree.ToCompact())
		} else {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(tree); err != nil {
				fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
				os.Exit(1)
			}
		}
		return
	}

	dedupe := !*noDedupe

	mode, err := seaportal.ParseLinkRetention(*linksMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *citations && *linksMode != "all" {
		fmt.Fprintln(os.Stderr, "warning: --citations ignored because --links is set explicitly (--links takes precedence)")
	}

	chunkCfg, err := seaportal.ParseChunkConfig(*chunk)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	opts := seaportal.Options{Dedupe: dedupe, NoNearDedupe: *noNearDedupe, FastMode: *fast, ProbeSearch: *probeSearch, MaxRetries: *retries, MaxRetryWait: *maxRetryWait, TotalRetryTimeout: *retryTimeout, WithLinks: *withLinks, WithImages: *withImages, WithTables: *withTables, WithComments: *withComments, Citations: *citations, LinkRetention: mode, Chunk: chunkCfg, SelectCSS: *selectCSS, StripCSS: *stripCSS, MaxTokens: *maxTokens, HeadOnly: *headOnly, RespectRobots: *respectRobots, UserAgent: *ua, NoPruneFallback: *noPruneFallback, RateLimit: *rateLimit, Proxy: *proxy, CacheDir: *cacheDir, CacheTTL: *cacheTTL, CacheStaleTolerance: *cacheStaleTolerance, NoCache: *noCache, NoPDF: *noPDF, SchemaPath: *schemaPath, Query: *query, TopN: *topN, FilterByQuery: *filterByQuery, SplitOut: *splitOut, SplitBytes: *splitBytes}
	var result seaportal.Result
	if stdinMode {
		result = seaportal.FromHTMLWithOptions(stdinHTML, targetURL, opts)
	} else {
		result = seaportal.FromURLWithOptions(targetURL, opts)
	}

	// Output splitting: write the rendered content to multiple files and emit a
	// manifest on stdout in place of the content body.
	if *splitOut != "" {
		format := "md"
		if *jsonOut {
			format = "json"
		}
		maxBytes := *splitBytes
		if maxBytes <= 0 && *maxTokens > 0 {
			maxBytes = *maxTokens * 4
		}
		files, err := seaportal.SplitResultToFiles(result, seaportal.SplitConfig{
			Dir:      *splitOut,
			MaxBytes: maxBytes,
			Format:   format,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "split error: %v\n", err)
			os.Exit(1)
		}
		result.SplitFiles = files
		for _, f := range files {
			fmt.Printf("%s\t%d/%d\t%d\n", f.Path, f.Index, f.Of, f.Bytes)
		}
		return
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *xmlOut {
		data, err := seaportal.ResultToTEIXML(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding XML: %v\n", err)
			os.Exit(1)
		}
		if _, err := os.Stdout.Write(data); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing XML: %v\n", err)
			os.Exit(1)
		}
		fmt.Println()
		return
	}

	var output strings.Builder
	output.WriteString("---\n")
	fmt.Fprintf(&output, "title: %q\n", result.Title)
	fmt.Fprintf(&output, "url: %s\n", result.URL)
	fmt.Fprintf(&output, "byline: %q\n", result.Byline)
	fmt.Fprintf(&output, "excerpt: %q\n", result.Excerpt)
	fmt.Fprintf(&output, "sitename: %q\n", result.SiteName)
	fmt.Fprintf(&output, "length: %d\n", result.Length)
	fmt.Fprintf(&output, "confidence: %d\n", result.Confidence)
	fmt.Fprintf(&output, "isSpa: %v\n", result.IsSPA)
	if len(result.SPASignals) > 0 {
		fmt.Fprintf(&output, "spaSignals: %v\n", result.SPASignals)
	}
	fmt.Fprintf(&output, "pageClass: %s\n", result.Profile.Class)
	fmt.Fprintf(&output, "outcome: %s\n", result.Profile.Outcome)
	fmt.Fprintf(&output, "trustworthy: %v\n", result.Profile.Trustworthy)
	if len(result.Profile.Reasons) > 0 {
		fmt.Fprintf(&output, "classReasons: %v\n", result.Profile.Reasons)
	}
	fmt.Fprintf(&output, "headings: %d\n", result.HeadingCount)
	fmt.Fprintf(&output, "links: %d\n", result.LinkCount)
	fmt.Fprintf(&output, "paragraphs: %d\n", result.ParagraphCount)
	if result.DedupeApplied {
		fmt.Fprintf(&output, "dedupeApplied: %v\n", result.DedupeApplied)
		fmt.Fprintf(&output, "duplicatesRemoved: %d\n", result.DuplicatesRemoved)
		if len(result.DuplicateSignals) > 0 {
			fmt.Fprintf(&output, "duplicateSignals: %v\n", result.DuplicateSignals)
		}
		if result.NearDuplicatesRemoved > 0 {
			fmt.Fprintf(&output, "nearDuplicatesRemoved: %d\n", result.NearDuplicatesRemoved)
		}
		if len(result.NearDuplicateSignals) > 0 {
			fmt.Fprintf(&output, "nearDuplicateSignals: %v\n", result.NearDuplicateSignals)
		}
	}
	fmt.Fprintf(&output, "validationOk: %v\n", result.Validation.IsValid)
	fmt.Fprintf(&output, "needsBrowser: %v\n", result.Validation.NeedsBrowser)
	fmt.Fprintf(&output, "validationConfidence: %.2f\n", result.Validation.Confidence)
	if len(result.Validation.Issues) > 0 {
		fmt.Fprintf(&output, "validationIssues: %v\n", result.Validation.Issues)
	}
	output.WriteString("---\n\n")
	output.WriteString(result.Content)

	// regression: cli-file-path-panic — guard against args without a `//`
	// separator (file paths, data: URIs, malformed input). Falls back to a
	// synthetic `local` slug so the renders/ filename is still valid.
	parts := strings.SplitN(targetURL, "//", 2)
	domain := "local"
	if len(parts) == 2 {
		domain = strings.ReplaceAll(parts[1], "/", "_")
	}
	if idx := strings.Index(domain, "/"); idx > 0 {
		domain = domain[:idx]
	}
	domain = strings.ReplaceAll(domain, ":", "_")
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join("renders", "seaportal", fmt.Sprintf("%s_%s.md", domain, timestamp))

	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create directory: %v\n", err)
	}
	if err := os.WriteFile(filename, []byte(output.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save markdown: %v\n", err)
	}

	jsonFile := filepath.Join("renders", "seaportal", fmt.Sprintf("%s_%s.json", domain, timestamp))
	jsonData, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(jsonFile, jsonData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save JSON: %v\n", err)
	}

	fmt.Printf("Saved: %s (%d bytes, %dms, confidence: %d%%)\n", filename, result.Length, result.TimeMs, result.Confidence)
	fmt.Printf("📋 Classification: %s\n", result.Profile.String())
	if len(result.Profile.Reasons) > 0 {
		fmt.Printf("   Reasons: %v\n", result.Profile.Reasons)
	}
	if result.IsSPA {
		fmt.Printf("⚠️  SPA detected: %v\n", result.SPASignals)
	}
	fmt.Println("\n--- Content ---")
	fmt.Println(output.String())
}

func fetchHTML(url string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
