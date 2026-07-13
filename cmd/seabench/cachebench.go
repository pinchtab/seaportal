package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/seaportal"
	"github.com/pinchtab/seaportal/internal/testserver/fixture"
)

type CacheBenchReport struct {
	Version    int                       `json:"version"`
	CapturedAt string                    `json:"captured_at"`
	GitSHA     string                    `json:"git_sha"`
	GoVersion  string                    `json:"go_version"`
	GOMAXPROCS int                       `json:"gomaxprocs"`
	N          int                       `json:"n"`
	HotRatio   float64                   `json:"hot_ratio"`
	HotURLs    int                       `json:"hot_urls"`
	ColdURLs   int                       `json:"cold_urls"`
	Seed       int64                     `json:"seed"`
	PerMode    map[string]CacheModeStats `json:"per_mode"`
	Notes      []string                  `json:"notes,omitempty"`
}

type CacheModeStats struct {
	HitRate      float64 `json:"hit_rate"`
	Hits         int     `json:"hits"`
	Requests     int     `json:"requests"`
	P50Ms        int64   `json:"p50_ms"`
	P95Ms        int64   `json:"p95_ms"`
	MeanRSSBytes uint64  `json:"mean_rss_bytes"`
	TotalMs      int64   `json:"total_ms"`
	Errors       int     `json:"errors"`
}

var cacheModeOrder = []string{"off", "ttl-24h", "swr-10m"}

func runCacheBench(args []string) {
	fs := flag.NewFlagSet("cachebench", flag.ExitOnError)
	n := fs.Int("n", 200, "Total requests per mode")
	hotRatio := fs.Float64("hot-ratio", 0.8, "Fraction of requests drawn from the hot pool (0..1)")
	output := fs.String("output", "tests/bench/reports", "Output directory for JSON + Markdown reports")
	hotCount := fs.Int("hot", 5, "Number of hot URLs")
	coldCount := fs.Int("cold", 50, "Number of cold URLs")
	seed := fs.Int64("seed", 42, "Deterministic seed for the weighted URL sampler")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if *n <= 0 {
		fmt.Fprintln(os.Stderr, "--n must be > 0")
		os.Exit(2)
	}
	if *hotRatio < 0 || *hotRatio > 1 {
		fmt.Fprintln(os.Stderr, "--hot-ratio must be in [0,1]")
		os.Exit(2)
	}
	if *hotCount <= 0 || *coldCount <= 0 {
		fmt.Fprintln(os.Stderr, "--hot and --cold must be > 0")
		os.Exit(2)
	}

	report := executeCacheBench(*n, *hotRatio, *hotCount, *coldCount, *seed)

	_, mdPath, err := emitReports(*output, "cachebench", report, renderCacheBenchMarkdown(report))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ttl := report.PerMode["ttl-24h"]
	swr := report.PerMode["swr-10m"]
	fmt.Printf("cachebench: ttl-24h hit=%.0f%% p50=%dms, swr-10m hit=%.0f%%. See %s\n",
		ttl.HitRate*100, ttl.P50Ms, swr.HitRate*100, mdPath)
}

func executeCacheBench(n int, hotRatio float64, hotCount, coldCount int, seed int64) CacheBenchReport {
	srv, hotPaths, coldPaths := newCacheBenchServer(hotCount, coldCount)
	defer srv.Close()

	hotURLs := joinURLs(srv.URL(), hotPaths)
	coldURLs := joinURLs(srv.URL(), coldPaths)

	sequence := generateURLSequence(n, hotRatio, hotURLs, coldURLs, seed)

	report := CacheBenchReport{
		Version:    1,
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		GitSHA:     gitSHA(),
		GoVersion:  runtime.Version(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
		N:          n,
		HotRatio:   hotRatio,
		HotURLs:    hotCount,
		ColdURLs:   coldCount,
		Seed:       seed,
		PerMode:    map[string]CacheModeStats{},
		Notes: []string{
			"swr-10m uses CacheTTL=24h + CacheStaleTolerance=10m. SWR only fires on EXPIRED entries; in a fresh-cache benchmark no entry ever expires inside the run, so swr-10m numerically behaves like ttl-24h. The mode is kept so a regression that breaks the SWR code path lights up here.",
			"hit-rate ceiling for ttl-24h/swr-10m ≈ hot_ratio - hot_urls/N (each hot URL pays one unavoidable first-miss).",
		},
	}

	for _, mode := range cacheModeOrder {
		report.PerMode[mode] = runCacheMode(mode, sequence)
	}
	return report
}

func runCacheMode(mode string, sequence []string) CacheModeStats {
	dir, err := os.MkdirTemp("", "seabench-cache-"+mode+"-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkdir cachedir:", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	opts := optionsForMode(mode, dir)
	n := len(sequence)
	lat := make([]time.Duration, n)
	heap := make([]uint64, n)
	hits := 0
	errors := 0

	var ms runtime.MemStats
	t0 := time.Now()
	for i, target := range sequence {
		iterStart := time.Now()
		res := seaportal.FromURLWithOptions(target, opts)
		lat[i] = time.Since(iterStart)
		if res.Error != "" {
			errors++
		}
		if res.CacheHit {
			hits++
		}
		runtime.ReadMemStats(&ms)
		heap[i] = ms.HeapInuse
	}
	total := time.Since(t0)

	var heapSum uint64
	for _, h := range heap {
		heapSum += h
	}
	var meanHeap uint64
	if n > 0 {
		meanHeap = heapSum / uint64(n)
	}

	return CacheModeStats{
		HitRate:      float64(hits) / float64(n),
		Hits:         hits,
		Requests:     n,
		P50Ms:        percentile(lat, 0.50).Milliseconds(),
		P95Ms:        percentile(lat, 0.95).Milliseconds(),
		MeanRSSBytes: meanHeap,
		TotalMs:      total.Milliseconds(),
		Errors:       errors,
	}
}

func optionsForMode(mode, cacheDir string) seaportal.Options {
	switch mode {
	case "off":
		return seaportal.Options{NoCache: true}
	case "ttl-24h":
		return seaportal.Options{CacheDir: cacheDir, CacheTTL: 24 * time.Hour}
	case "swr-10m":
		return seaportal.Options{
			CacheDir:            cacheDir,
			CacheTTL:            24 * time.Hour,
			CacheStaleTolerance: 10 * time.Minute,
		}
	default:
		return seaportal.Options{NoCache: true}
	}
}

func newCacheBenchServer(hotCount, coldCount int) (*fixture.Server, []string, []string) {
	srv := fixture.New()
	total := hotCount + coldCount
	hot := make([]string, 0, hotCount)
	cold := make([]string, 0, coldCount)
	for i := 0; i < total; i++ {
		path := "/page/" + strconv.Itoa(i)
		body := syntheticHTML(i)
		srv.Route("GET", path, fixture.Body(body, "text/html; charset=utf-8"))
		if i < hotCount {
			hot = append(hot, path)
		} else {
			cold = append(cold, path)
		}
	}
	return srv, hot, cold
}

func syntheticHTML(i int) []byte {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Page `)
	b.WriteString(strconv.Itoa(i))
	b.WriteString(`</title><meta name="description" content="Synthetic page `)
	b.WriteString(strconv.Itoa(i))
	b.WriteString(` for cachebench."></head><body><article><h1>Page `)
	b.WriteString(strconv.Itoa(i))
	b.WriteString(`</h1>`)
	for p := 0; p < 6; p++ {
		b.WriteString(`<p>Paragraph `)
		b.WriteString(strconv.Itoa(p))
		b.WriteString(` of page `)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`. Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p>`)
	}
	b.WriteString(`</article></body></html>`)
	return []byte(b.String())
}

func generateURLSequence(n int, hotRatio float64, hot, cold []string, seed int64) []string {
	r := rand.New(rand.NewSource(seed))
	out := make([]string, n)
	for i := 0; i < n; i++ {
		if r.Float64() < hotRatio && len(hot) > 0 {
			out[i] = hot[r.Intn(len(hot))]
		} else {
			out[i] = cold[r.Intn(len(cold))]
		}
	}
	return out
}

func joinURLs(base string, paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = base + p
	}
	return out
}

func renderCacheBenchMarkdown(r CacheBenchReport) string {
	var b strings.Builder
	reportHeader(&b, "SeaPortal Cache Bench Report",
		"Captured", r.CapturedAt,
		"Git SHA", "`"+r.GitSHA+"`")
	fmt.Fprintf(&b, "- Go: %s, GOMAXPROCS=%d\n", r.GoVersion, r.GOMAXPROCS)
	fmt.Fprintf(&b, "- N: %d, hot_ratio: %.2f, hot_urls: %d, cold_urls: %d, seed: %d\n\n",
		r.N, r.HotRatio, r.HotURLs, r.ColdURLs, r.Seed)

	fmt.Fprintln(&b, "## Per-mode")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "| Mode | Hit rate | Hits / N | p50 (ms) | p95 (ms) | Mean HeapInuse | Total (ms) | Errors |")
	fmt.Fprintln(&b, "|---|---|---|---|---|---|---|---|")
	for _, mode := range cacheModeOrder {
		s := r.PerMode[mode]
		fmt.Fprintf(&b, "| %s | %.4f | %d / %d | %d | %d | %d | %d | %d |\n",
			mode, s.HitRate, s.Hits, s.Requests, s.P50Ms, s.P95Ms, s.MeanRSSBytes, s.TotalMs, s.Errors)
	}
	fmt.Fprintln(&b, "")

	if len(r.Notes) > 0 {
		fmt.Fprintln(&b, "## Notes")
		fmt.Fprintln(&b, "")
		for _, note := range r.Notes {
			fmt.Fprintf(&b, "- %s\n", note)
		}
		fmt.Fprintln(&b, "")
	}

	return b.String()
}
