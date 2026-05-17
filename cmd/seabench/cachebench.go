package main

// cachebench is the cache hit-rate / latency lane of seabench. It drives a
// deterministic 80/20 mixed-traffic pattern (80% requests against 5 "hot"
// URLs, 20% against 50 "cold" URLs) through three cache modes — `off`,
// `ttl-24h`, and `swr-10m` — and reports per-mode hit rate, p50/p95 latency,
// and mean HeapInuse.
//
// Why a separate command (not folded into `stress`):
//   - `stress` hammers a SINGLE URL to surface allocation / GC regressions;
//     it deliberately collapses URL variety so the engine work is the signal.
//   - `cachebench` REQUIRES URL variety (hot/cold split) so the cache earns
//     its complexity in measurable hit-rate numbers.
//
// Why three modes:
//   - `off`            (NoCache=true)            — baseline; hit-rate must be 0.
//   - `ttl-24h`        (CacheTTL=24h)            — TTL-only behaviour.
//   - `swr-10m`        (CacheTTL=24h + SWR=10m)  — proves the SWR knob does
//     not regress hit-rate in a fresh-cache workload. SWR only fires on
//     EXPIRED entries; with a fresh-tempdir cache no entry ever expires
//     inside the run, so `swr-10m` numerically behaves like `ttl-24h`. The
//     mode is kept in the bench so a future regression that breaks the SWR
//     code path (e.g. always-stale classification) lights up here.
//
// Why a fresh tempdir per mode:
//   - Cross-mode contamination would make hit-rate meaningless. Each mode
//     starts from a cold cache and is responsible for warming its own hot
//     URLs across the run.
//
// Why deterministic sampling (rand.NewSource(42)):
//   - Reproducible hit-rate numbers across runs. The 80/20 weighted draw
//     happens with the SAME seed for every mode so each mode sees the
//     identical URL sequence.
//
// Expected hit-rate ceiling for `ttl-24h` / `swr-10m`:
//   - 5 hot URLs, each first-touch is a miss (5 unavoidable cold misses).
//     Worst case: 5/200 = 2.5% guaranteed misses, plus the 20% cold traffic
//     (each cold URL repeats rarely so most cold draws are also misses).
//     Realistic ceiling ≈ hot_share - 5/N ≈ 0.80 - 0.025 = ~0.775.

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/seaportal/internal/engine"
	"github.com/pinchtab/seaportal/internal/testserver/fixture"
)

// CacheBenchReport is the on-disk JSON shape (version 1). snake_case keys
// for cross-tool readability (jq, dashboards).
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

// CacheModeStats is the per-mode aggregate written into PerMode.
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

// cacheModeOrder fixes report column order independent of map iteration.
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

	if err := os.MkdirAll(*output, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir output:", err)
		os.Exit(1)
	}
	ts := time.Now().UTC().Format("20060102-150405")
	jsonPath := filepath.Join(*output, fmt.Sprintf("cachebench_%s.json", ts))
	mdPath := filepath.Join(*output, fmt.Sprintf("cachebench_%s.md", ts))

	if err := writeCacheBenchJSON(jsonPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "write json:", err)
		os.Exit(1)
	}
	if err := atomicWrite(mdPath, renderCacheBenchMarkdown(report)); err != nil {
		fmt.Fprintln(os.Stderr, "write markdown:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", jsonPath)
	fmt.Println("wrote", mdPath)
}

// executeCacheBench builds the fixture server, drives N requests through each
// of the three cache modes with a fresh tempdir per mode, and returns the
// populated report. Extracted from runCacheBench so tests can call it
// directly without intercepting os.Exit.
func executeCacheBench(n int, hotRatio float64, hotCount, coldCount int, seed int64) CacheBenchReport {
	srv, hotPaths, coldPaths := newCacheBenchServer(hotCount, coldCount)
	defer srv.Close()

	hotURLs := joinURLs(srv.URL(), hotPaths)
	coldURLs := joinURLs(srv.URL(), coldPaths)

	// Generate the URL sequence ONCE with the given seed so every mode sees
	// the identical traffic pattern. This isolates the variable under test
	// (cache mode) from sampling noise.
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

// runCacheMode executes the URL sequence against a single cache mode,
// returning the per-mode aggregate. A fresh tempdir is created and torn
// down per mode so hit-rate is honest.
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
		res := engine.FromURLWithOptions(target, opts)
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
		P50Ms:        percentileMs(lat, 50),
		P95Ms:        percentileMs(lat, 95),
		MeanRSSBytes: meanHeap,
		TotalMs:      total.Milliseconds(),
		Errors:       errors,
	}
}

// optionsForMode returns the engine.Options for the given cache mode. The
// `off` mode forces NoCache=true so the engine bypasses the disk cache
// entirely; the two cached modes share a per-request CacheDir.
func optionsForMode(mode, cacheDir string) engine.Options {
	switch mode {
	case "off":
		return engine.Options{NoCache: true}
	case "ttl-24h":
		return engine.Options{CacheDir: cacheDir, CacheTTL: 24 * time.Hour}
	case "swr-10m":
		return engine.Options{
			CacheDir:            cacheDir,
			CacheTTL:            24 * time.Hour,
			CacheStaleTolerance: 10 * time.Minute,
		}
	default:
		return engine.Options{NoCache: true}
	}
}

// newCacheBenchServer spins up a fixture server with `hot+cold` routes at
// /page/N. Returns the server plus the two slices of route paths so the
// caller can build URLs. Each route serves a small, distinct HTML body so
// the engine has a real extract to perform and the cache key (URL-based)
// never collides.
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

// syntheticHTML produces a small, deterministic HTML body for route id `i`.
// Body is ~1-2KB — enough work for the extract pipeline to produce a real
// result without dominating the cache-vs-fetch latency signal.
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

// generateURLSequence draws n URLs using a deterministic 80/20 weighted
// sampler: each draw, with probability hotRatio pick a uniform hot URL,
// else pick a uniform cold URL. Same seed → identical sequence across
// modes, which makes per-mode hit-rate directly comparable.
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

func writeCacheBenchJSON(path string, r CacheBenchReport) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, string(raw)+"\n")
}

func renderCacheBenchMarkdown(r CacheBenchReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# SeaPortal Cache Bench Report\n\n")
	fmt.Fprintf(&b, "- Captured: %s\n", r.CapturedAt)
	fmt.Fprintf(&b, "- Git SHA: `%s`\n", r.GitSHA)
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
