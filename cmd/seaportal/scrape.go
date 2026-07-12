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

// runScrape implements `seaportal scrape <base-url> [flags]`: it maps the spec
// flags onto ScrapeOptions, runs the full pipeline, and emits the result per
// --output (json | md | directory).
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
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: seaportal scrape <base-url> [flags]")
		fs.PrintDefaults()
	}
	// Support both `scrape [flags] <url>` and the spec's `scrape <url> [flags]`:
	// stdlib flag stops at the first positional, so pull the base URL out and
	// parse any flags that followed it.
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

	// Secure-by-default fetch policy, mirroring the sitemap/feed verbs:
	// --allow-internal lifts only the private-IP block.
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
	}

	res, err := seaportal.ScrapeSite(ctx, opts)
	if err != nil && res == nil {
		fmt.Fprintln(os.Stderr, "scrape error:", err)
		os.Exit(1)
	}
	if err != nil {
		// Interrupted mid-run (Ctrl-C / caller deadline): ScrapeSite returned
		// the partial result alongside ctx.Err() (audit T21) — render what
		// was scraped, warn, and exit non-zero to signal the interruption.
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

// renderScrapeResult emits res on stdout in the chosen output format
// (markdown digest, directory of pages, or JSON — the default).
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
	default: // json
		data, err := seaportal.RenderScrapeJSON(res)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	}
	return nil
}
