package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pinchtab/seaportal"
)

const (
	cliDefaultRetries      = 3
	cliDefaultMaxRetryWait = 30 * time.Second
	cliDefaultRetryTimeout = 90 * time.Second
)

type extractFlags struct {
	noDedupe        *bool
	noNearDedupe    *bool
	fast            *bool
	probeSearch     *bool
	withLinks       *bool
	withImages      *bool
	withTables      *bool
	withComments    *bool
	citations       *bool
	linksMode       *string
	chunk           *string
	selectCSS       *string
	stripCSS        *string
	headOnly        *bool
	noPruneFallback *bool
	schemaPath      *string
	query           *string
	topN            *int
	filterByQuery   *bool
	maxTokens       *int

	jsonOut        *bool
	xmlOut         *bool
	snapshot       *bool
	snapshotFilter *string
	snapshotFormat *string
	splitOut       *string
	splitBytes     *int
	saveDir        *string

	retries       *int
	maxRetryWait  *time.Duration
	retryTimeout  *time.Duration
	respectRobots *bool
	rateLimit     *time.Duration
	ua            *string
	baseURL       *string
	proxy         *string
	noPDF         *bool

	cacheDir            *string
	cacheTTL            *time.Duration
	cacheStaleTolerance *time.Duration
	noCache             *bool

	blockPrivateIPs      *bool
	allowInternal        *bool
	maxRedirects         *int
	allowDomains         *string
	denyDomains          *string
	trustedResolveCIDRs  *string
	maxResponseBytes     *int64
	maxDecompressedBytes *int64

	showVersion *bool
}

func registerExtractFlags(cli *flag.FlagSet) *extractFlags {
	f := &extractFlags{}
	f.noDedupe = cli.Bool("no-dedupe", false, "Disable deduplication (enabled by default)")
	f.noNearDedupe = cli.Bool("no-near-dedupe", false, "Disable simhash near-duplicate detection (enabled by default)")
	f.fast = cli.Bool("fast", false, "Fast mode: bail early if browser is needed")
	f.probeSearch = cli.Bool("probe-search", false, "Override outcome to needs-browser when a search URL returns no result-list structure")
	f.jsonOut = cli.Bool("json", false, "Output JSON instead of Markdown")
	f.xmlOut = cli.Bool("xml", false, "Output TEI-Lite XML instead of Markdown")
	f.snapshot = cli.Bool("snapshot", false, "Output accessibility tree as JSON (deprecated: use 'seaportal snapshot <url>')")
	f.snapshotFilter = cli.String("filter", "", "Snapshot filter: 'interactive' to show only interactive elements")
	f.snapshotFormat = cli.String("format", "json", "Snapshot format: 'json' or 'compact'")
	f.maxTokens = cli.Int("max-tokens", 0, "Approximate token limit for output (snapshot tree OR Markdown body; 0 = unlimited)")
	f.retries = cli.Int("retries", cliDefaultRetries, "Max retry attempts for transient failures (502/503/504/429)")
	f.maxRetryWait = cli.Duration("max-retry-wait", cliDefaultMaxRetryWait, "Max single backoff wait")
	f.retryTimeout = cli.Duration("retry-timeout", cliDefaultRetryTimeout, "Total budget for all retries")
	f.withLinks = cli.Bool("with-links", false, "Emit list of discovered <a> links with text + rel")
	f.withImages = cli.Bool("with-images", false, "Emit list of discovered <img> entries with src/alt/srcset")
	f.withTables = cli.Bool("with-tables", false, "Emit structured tables (caption/headers/rows) in result")
	f.withComments = cli.Bool("with-comments", false, "Emit user-generated comments separately in Result.Comments")
	f.citations = cli.Bool("citations", false, "Convert inline links to numbered references with a References section at the bottom (synonym for --links=footer)")
	f.linksMode = cli.String("links", "all", "Markdown link retention: none|text|all|footer")
	f.chunk = cli.String("chunk", "", "Chunking strategy: heading | sentence[:SIZE] | window[:SIZE[:OVERLAP]]")
	f.selectCSS = cli.String("select", "", "CSS selector(s) to scope extraction (comma-separated)")
	f.stripCSS = cli.String("strip", "", "CSS selector(s) to remove before extraction (comma-separated)")
	f.headOnly = cli.Bool("head-only", false, "Fetch only the first 16 KB and extract metadata + canonical (no body extraction)")
	f.noPruneFallback = cli.Bool("no-prune-fallback", false, "Disable the tag-density heuristic fallback when readability output looks thin")
	f.respectRobots = cli.Bool("respect-robots", false, "Consult robots.txt and refuse to fetch disallowed paths")
	f.rateLimit = cli.Duration("rate-limit", 0, "Minimum interval between requests to the same host (e.g. 500ms, 2s)")
	f.ua = cli.String("ua", "", "User-Agent: preset name (chrome|safari|firefox|googlebot|bingbot|seaportal|search-bot) or literal UA string")
	f.baseURL = cli.String("base-url", "", "Base URL for stdin HTML input (used to resolve relative links and host-aware checks)")
	f.proxy = cli.String("proxy", "", "Proxy URL: http://user:pass@host:port, https://..., or socks5://...")
	f.cacheDir = cli.String("cache", "", "Enable on-disk cache (give a directory path)")
	f.cacheTTL = cli.Duration("cache-ttl", 24*time.Hour, "Cache freshness window (e.g. 1h, 24h)")
	f.cacheStaleTolerance = cli.Duration("cache-stale-tolerance", 0, "SWR: serve cached entries stale (background revalidate) within TTL+tolerance window")
	f.noCache = cli.Bool("no-cache", false, "Bypass cache reads (writes still happen if --cache is set)")
	f.noPDF = cli.Bool("no-pdf", false, "Skip PDF extraction (treat application/pdf as binary, original pre-PDF behaviour)")
	f.schemaPath = cli.String("schema", "", "Path to a CSS schema (JSON/YAML) to extract structured data into result.schema")
	f.query = cli.String("query", "", "Score sections by BM25 relevance to this query")
	f.topN = cli.Int("top-n", 0, "Keep only the top-N most relevant sections (0 = all)")
	f.filterByQuery = cli.Bool("filter-by-query", false, "Replace Content with concatenated top-N sections (default top-3 when --top-n is unset)")
	f.splitOut = cli.String("split-out", "", "Directory to write split output files into")
	f.splitBytes = cli.Int("split-bytes", 0, "Approximate bytes per split file (default: --max-tokens × 4 or 32768)")
	f.saveDir = cli.String("save-dir", "", "Also write the rendered Markdown + JSON to <dir>/<domain>_<timestamp>.{md,json} (default: stdout only, no files)")

	f.blockPrivateIPs = cli.Bool("block-private-ips", true, "SSRF guard: reject targets resolving to private/internal IPs")
	f.allowInternal = cli.Bool("allow-internal", false, "Escape hatch: allow private/internal IP targets (turns off --block-private-ips)")
	cli.BoolVar(f.allowInternal, "allow-private-ips", false, "Alias for --allow-internal")
	f.maxRedirects = cli.Int("max-redirects", 10, "Max redirect hops (0 = none, -1 = unlimited)")
	f.allowDomains = cli.String("allow-domains", "", "Comma-separated host allowlist (suffix match); empty = allow any")
	f.denyDomains = cli.String("deny-domains", "", "Comma-separated host blocklist (suffix match)")
	f.trustedResolveCIDRs = cli.String("trusted-resolve-cidrs", "", "Comma-separated CIDRs/IPs allowed to resolve to non-public addresses")
	f.maxResponseBytes = cli.Int64("max-response-bytes", 50<<20, "Max raw response body bytes (0 = unlimited)")
	f.maxDecompressedBytes = cli.Int64("max-decompressed-bytes", 200<<20, "Max decompressed body bytes (0 = unlimited)")

	f.showVersion = cli.Bool("version", false, "Show version")
	cli.BoolVar(f.showVersion, "v", false, "Show version")
	return f
}

func (f *extractFlags) securityPolicy() *seaportal.SecurityPolicy {
	return &seaportal.SecurityPolicy{
		BlockPrivateIPs:      *f.blockPrivateIPs && !*f.allowInternal,
		AllowedSchemes:       []string{"http", "https"},
		MaxRedirects:         *f.maxRedirects,
		RevalidateRedirects:  true,
		MaxResponseBytes:     *f.maxResponseBytes,
		MaxDecompressedBytes: *f.maxDecompressedBytes,
		AllowedDomains:       splitCSV(*f.allowDomains),
		DeniedDomains:        splitCSV(*f.denyDomains),
		TrustedResolveCIDRs:  splitCSV(*f.trustedResolveCIDRs),
	}
}

type outputConfig struct {
	json       bool
	xml        bool
	splitOut   string
	splitBytes int
	maxTokens  int
	saveDir    string
}

func buildExtractOptions(f *extractFlags) (seaportal.Options, outputConfig, error) {
	mode, err := seaportal.ParseLinkRetention(*f.linksMode)
	if err != nil {
		return seaportal.Options{}, outputConfig{}, err
	}
	if *f.citations && *f.linksMode != "all" {
		fmt.Fprintln(os.Stderr, "warning: --citations ignored because --links is set explicitly (--links takes precedence)")
	}
	chunkCfg, err := seaportal.ParseChunkConfig(*f.chunk)
	if err != nil {
		return seaportal.Options{}, outputConfig{}, err
	}

	opts := seaportal.Options{
		Dedupe:              !*f.noDedupe,
		NoNearDedupe:        *f.noNearDedupe,
		FastMode:            *f.fast,
		ProbeSearch:         *f.probeSearch,
		MaxRetries:          *f.retries,
		MaxRetryWait:        *f.maxRetryWait,
		TotalRetryTimeout:   *f.retryTimeout,
		WithLinks:           *f.withLinks,
		WithImages:          *f.withImages,
		WithTables:          *f.withTables,
		WithComments:        *f.withComments,
		Citations:           *f.citations,
		LinkRetention:       mode,
		Chunk:               chunkCfg,
		SelectCSS:           *f.selectCSS,
		StripCSS:            *f.stripCSS,
		MaxTokens:           *f.maxTokens,
		HeadOnly:            *f.headOnly,
		RespectRobots:       *f.respectRobots,
		UserAgent:           *f.ua,
		NoPruneFallback:     *f.noPruneFallback,
		RateLimit:           *f.rateLimit,
		Proxy:               *f.proxy,
		CacheDir:            *f.cacheDir,
		CacheTTL:            *f.cacheTTL,
		CacheStaleTolerance: *f.cacheStaleTolerance,
		NoCache:             *f.noCache,
		NoPDF:               *f.noPDF,
		SchemaPath:          *f.schemaPath,
		Query:               *f.query,
		TopN:                *f.topN,
		FilterByQuery:       *f.filterByQuery,
		SplitOut:            *f.splitOut,
		SplitBytes:          *f.splitBytes,
		Security:            f.securityPolicy(),
	}
	cfg := outputConfig{
		json:       *f.jsonOut,
		xml:        *f.xmlOut,
		splitOut:   *f.splitOut,
		splitBytes: *f.splitBytes,
		maxTokens:  *f.maxTokens,
		saveDir:    *f.saveDir,
	}
	return opts, cfg, nil
}

func runExtract(ctx context.Context, rawArgs []string) {
	cli := flag.NewFlagSet("seaportal", flag.ExitOnError)
	f := registerExtractFlags(cli)

	usage := func(w io.Writer) {
		_, _ = fmt.Fprint(w, "SeaPortal - Extract clean Markdown from URLs with SPA detection\n\nUsage:\n  seaportal [options] <url>\n\nOptions:\n")
		cli.SetOutput(w)
		cli.PrintDefaults()
		cli.SetOutput(os.Stderr)
	}
	cli.Usage = func() { usage(os.Stderr) }

	for _, a := range rawArgs {
		if a == "--" || !strings.HasPrefix(a, "-") {
			break
		}
		if a == "-h" || a == "--h" || a == "-help" || a == "--help" {
			usage(os.Stdout)
			return
		}
	}

	_ = cli.Parse(rawArgs)

	if *f.showVersion {
		fmt.Printf("seaportal %s\n", version)
		return
	}

	if *f.xmlOut && *f.jsonOut {
		fmt.Fprintln(os.Stderr, "error: --xml and --json are mutually exclusive")
		os.Exit(2)
	}
	if *f.splitOut != "" && *f.xmlOut {
		fmt.Fprintln(os.Stderr, "error: --split-out is not supported with --xml")
		os.Exit(2)
	}

	targetURL, stdinHTML, stdinMode := resolveExtractInput(cli, f)

	if *f.snapshot {
		htmlContent := stdinHTML
		if !stdinMode {
			h, err := fetchHTML(ctx, targetURL, f.securityPolicy())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error fetching URL: %v\n", err)
				os.Exit(1)
			}
			htmlContent = h
		}
		if err := renderSnapshot(os.Stdout, htmlContent, *f.snapshotFilter, *f.snapshotFormat, *f.maxTokens); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	opts, outCfg, err := buildExtractOptions(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var result seaportal.Result
	if stdinMode {
		result = seaportal.FromHTMLWithOptions(stdinHTML, targetURL, opts)
	} else {
		result = seaportal.FromURLContext(ctx, targetURL, opts)
	}

	if err := renderResult(os.Stdout, &result, outCfg, targetURL); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func resolveExtractInput(cli *flag.FlagSet, f *extractFlags) (targetURL, stdinHTML string, stdinMode bool) {
	args := cli.Args()
	stdinMode = len(args) == 0 || (len(args) == 1 && args[0] == "-")
	if !stdinMode {
		return args[0], "", false
	}
	if *f.baseURL == "" {
		if len(args) == 0 {
			cli.Usage()
			os.Exit(2)
		}
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

	if *f.headOnly {
		fmt.Fprintln(os.Stderr, "warning: --head-only ignored in stdin mode")
		*f.headOnly = false
	}
	if *f.respectRobots {
		fmt.Fprintln(os.Stderr, "warning: --respect-robots ignored in stdin mode")
		*f.respectRobots = false
	}
	if *f.retries != cliDefaultRetries {
		fmt.Fprintln(os.Stderr, "warning: --retries ignored in stdin mode")
	}
	return *f.baseURL, string(htmlBytes), true
}

func renderResult(w io.Writer, result *seaportal.Result, cfg outputConfig, targetURL string) error {
	switch {
	case cfg.splitOut != "":
		return renderSplitFiles(w, result, cfg)
	case cfg.json:
		return renderJSON(w, result)
	case cfg.xml:
		return renderTEIXML(w, result)
	default:
		return renderMarkdown(w, result, cfg.saveDir, targetURL)
	}
}

func renderSplitFiles(w io.Writer, result *seaportal.Result, cfg outputConfig) error {
	format := "md"
	if cfg.json {
		format = "json"
	}
	maxBytes := cfg.splitBytes
	if maxBytes <= 0 && cfg.maxTokens > 0 {
		maxBytes = cfg.maxTokens * 4
	}
	files, err := seaportal.SplitResultToFiles(*result, seaportal.SplitConfig{
		Dir:      cfg.splitOut,
		MaxBytes: maxBytes,
		Format:   format,
	})
	if err != nil {
		return fmt.Errorf("split error: %v", err)
	}
	result.SplitFiles = files
	for _, f := range files {
		if _, err := fmt.Fprintf(w, "%s\t%d/%d\t%d\n", f.Path, f.Index, f.Of, f.Bytes); err != nil {
			return err
		}
	}
	return nil
}

func renderJSON(w io.Writer, result *seaportal.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}

func renderTEIXML(w io.Writer, result *seaportal.Result) error {
	data, err := seaportal.ResultToTEIXML(*result)
	if err != nil {
		return fmt.Errorf("encoding XML: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("writing XML: %w", err)
	}
	_, _ = fmt.Fprintln(w)
	return nil
}

func renderMarkdown(w io.Writer, result *seaportal.Result, saveDir, targetURL string) error {
	doc := markdownDocument(result)
	if saveDir == "" {
		_, _ = fmt.Fprintln(w, doc)
		return nil
	}

	domain := renderSlug(targetURL)
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(saveDir, fmt.Sprintf("%s_%s.md", domain, timestamp))
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create directory: %v\n", err)
	}
	if err := os.WriteFile(filename, []byte(doc), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save markdown: %v\n", err)
	}

	jsonFile := filepath.Join(saveDir, fmt.Sprintf("%s_%s.json", domain, timestamp))
	jsonData, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(jsonFile, jsonData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save JSON: %v\n", err)
	}

	_, _ = fmt.Fprintf(w, "Saved: %s (%d bytes, %dms, confidence: %d%%)\n", filename, result.Length, result.TimeMs, result.Confidence)
	_, _ = fmt.Fprintf(w, "📋 Classification: %s\n", result.Profile.String())
	if len(result.Profile.Reasons) > 0 {
		_, _ = fmt.Fprintf(w, "   Reasons: %v\n", result.Profile.Reasons)
	}
	if result.IsSPA {
		_, _ = fmt.Fprintf(w, "⚠️  SPA detected: %v\n", result.SPASignals)
	}
	_, _ = fmt.Fprintln(w, "\n--- Content ---")
	_, _ = fmt.Fprintln(w, doc)
	return nil
}

func markdownDocument(result *seaportal.Result) string {
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
	return output.String()
}

func renderSlug(targetURL string) string {
	parts := strings.SplitN(targetURL, "//", 2)
	domain := "local"
	if len(parts) == 2 {
		domain = strings.ReplaceAll(parts[1], "/", "_")
	}
	if idx := strings.Index(domain, "/"); idx > 0 {
		domain = domain[:idx]
	}
	return strings.ReplaceAll(domain, ":", "_")
}
