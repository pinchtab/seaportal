package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pinchtab/seaportal"
)

func runScrape(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("scrape", flag.ExitOnError)
	maxPages := fs.Int("max-pages", 50, "Maximum total pages to process")
	maxPerPattern := fs.Int("max-per-pattern", 8, "Max samples per URL pattern")
	full := fs.Bool("full", false, "Disable sampling (fetch all discovered pages)")
	includePatterns := fs.String("include-patterns", "", "Comma-separated glob patterns to include")
	excludePatterns := fs.String("exclude-patterns", "", "Comma-separated glob patterns to exclude")
	sampleStrategy := fs.String("sample-strategy", "balanced", "Sampling strategy: random|priority|balanced")
	output := fs.String("output", "json", "Output format: json|md|directory")
	jsonOut := fs.Bool("json", false, "Shorthand for --output json (matches the root command)")
	outDir := fs.String("out-dir", "", "Target directory for --output directory")
	withPerformance := fs.Bool("with-performance", false, "Include per-page performance data")
	respectRobots := fs.Bool("respect-robots", true, "Respect robots.txt disallow + crawl-delay")
	timeout := fs.Duration("timeout", 60*time.Second, "Overall scrape timeout")
	userAgent := fs.String("user-agent", "", "Override the User-Agent header")
	allowInternal := fs.Bool("allow-internal", false, "Allow private/internal IP targets")
	recentDays := fs.Int("recent-days", 0, "Only discover sitemap URLs modified within the last N days (0 = no limit). Essential for large news archives.")
	preview := fs.Bool("preview", false, "Preview the site tree: 1 sample per URL pattern with per-group counts, scoped to recent content, so you can decide which branches to expand")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: seaportal scrape <base-url> [flags]")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "scrape error: missing <base-url>")
		fs.Usage()
		os.Exit(2)
	}
	baseURL := rest[0]
	if len(rest) > 1 {
		_ = fs.Parse(rest[1:])
	}

	if *jsonOut && *output != "json" {
		fmt.Fprintf(os.Stderr, "scrape error: --json conflicts with --output %s\n", *output)
		os.Exit(2)
	}

	if *preview {
		*maxPerPattern = 1
		*full = false
		if *recentDays == 0 {
			*recentDays = 7
		}
	}
	if *recentDays < 0 {
		fmt.Fprintln(os.Stderr, "scrape error: --recent-days must be >= 0")
		os.Exit(2)
	}
	var since time.Time
	if *recentDays > 0 {
		since = time.Now().AddDate(0, 0, -*recentDays)
	}

	strategy := seaportal.SampleStrategy(*sampleStrategy)
	switch strategy {
	case seaportal.SampleBalanced, seaportal.SampleRandom, seaportal.SamplePriority:
	default:
		fmt.Fprintf(os.Stderr, "scrape error: unknown --sample-strategy %q (want balanced|random|priority)\n", *sampleStrategy)
		os.Exit(2)
	}

	out := seaportal.OutputFormat(*output)
	switch out {
	case seaportal.OutputJSON, seaportal.OutputMarkdown, seaportal.OutputDirectory:
	default:
		fmt.Fprintf(os.Stderr, "scrape error: unknown --output %q (want json|md|directory)\n", *output)
		os.Exit(2)
	}
	if out == seaportal.OutputDirectory && strings.TrimSpace(*outDir) == "" {
		fmt.Fprintln(os.Stderr, "scrape error: --output directory requires --out-dir <path>")
		os.Exit(2)
	}

	sec := seaportal.DefaultSecurityPolicy()
	if *allowInternal {
		sec.BlockPrivateIPs = false
	}

	robots := *respectRobots
	opts := &seaportal.ScrapeOptions{
		BaseURL:         baseURL,
		MaxPages:        *maxPages,
		MaxPerPattern:   *maxPerPattern,
		Full:            *full,
		IncludePatterns: splitCSV(*includePatterns),
		ExcludePatterns: splitCSV(*excludePatterns),
		SampleStrategy:  strategy,
		Output:          out,
		WithPerformance: *withPerformance,
		RespectRobots:   &robots,
		Timeout:         *timeout,
		UserAgent:       *userAgent,
		Security:        sec,
		Since:           since,
	}

	res, err := seaportal.ScrapeSite(ctx, opts)
	if err != nil && res == nil {
		fmt.Fprintln(os.Stderr, "scrape error:", err)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "scrape warning: interrupted (%v); rendering partial results\n", err)
	}

	if renderErr := renderScrapeResult(res, out, *outDir); renderErr != nil {
		fmt.Fprintln(os.Stderr, "scrape error:", renderErr)
		os.Exit(1)
	}
	if err != nil {
		os.Exit(1)
	}
}

func renderScrapeResult(res *seaportal.ScrapeResult, out seaportal.OutputFormat, outDir string) error {
	switch out {
	case seaportal.OutputMarkdown:
		fmt.Print(seaportal.RenderScrapeMarkdown(res))
	case seaportal.OutputDirectory:
		files, err := seaportal.WriteScrapeDirectory(res, outDir)
		if err != nil {
			return err
		}
		fmt.Printf("wrote %d pages to %s\n", len(files), outDir)
	default:
		data, err := seaportal.RenderScrapeJSON(res)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	}
	return nil
}
