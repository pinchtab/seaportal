package main

// stress is the throughput + memory-growth lane of seabench. It fires N
// sequential FromURL calls against an in-process fixture server hosting a
// single representative HTML page, samples runtime.MemStats.HeapInuse on
// every iteration, and writes both a JSON and a Markdown report.
//
// Why a separate command instead of folding into `eval`:
//   - `eval` is precision/recall/F1 quality bake-off, single-shot per fixture.
//   - `stress` is sustained-throughput + memory-growth regression detection;
//     it deliberately repeats the SAME fetch so allocation patterns and GC
//     pressure are the dominant signal, not fixture variety.
//
// Why sequential (v1):
//   - Concurrent fetches make the latency signal noisier and entangle the
//     measurement with the http.Transport pool and Go scheduler. Sequential
//     keeps the per-iteration cost legible. Concurrency lane is a follow-up.
//
// Why HeapInuse (not Sys):
//   - HeapInuse is the live heap the engine is actually holding. Sys also
//     counts mmap'd-but-unused regions Go has not returned to the OS yet,
//     which makes it lag-y and not actionable for regression detection.
//
// Why no runtime.GC() between iterations:
//   - We want to observe REAL-WORLD allocation rates including STW pauses,
//     not idealised post-GC numbers.
//
// Report JSON schema (version 1):
//
//	{
//	  "version": 1,
//	  "captured_at": "2026-05-17T12:00:00Z",
//	  "git_sha": "...",
//	  "go_version": "go1.25",
//	  "gomaxprocs": 8,
//	  "preset": "quick",
//	  "n": 50,
//	  "fixture": "testdata/index/text-npr.html",
//	  "total_elapsed_ms": 425,
//	  "urls_per_sec": 117.6,
//	  "latency_ms": {"p50": 7, "p95": 18, "p99": 30},
//	  "memory_bytes": {"start_heap": ..., "end_heap": ..., "peak_heap": ..., "growth": ...},
//	  "success_rate": 1.0,
//	  "errors": 0
//	}

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/pinchtab/seaportal/internal/engine"
	"github.com/pinchtab/seaportal/internal/testserver/fixture"
)

// presets maps the human-facing preset name to its iteration count. Chosen
// so `quick` runs in ~1-2s on a developer laptop (kept in the default unit
// suite), while `large` provides enough samples for percentile stability.
var presets = map[string]int{
	"quick":  50,
	"small":  200,
	"medium": 500,
	"large":  2000,
}

// StressReport mirrors the on-disk JSON schema. Field tags use snake_case
// for cross-tool readability (jq, dashboards). All fields are unconditionally
// populated so report diffs stay column-aligned.
type StressReport struct {
	Version        int     `json:"version"`
	CapturedAt     string  `json:"captured_at"`
	GitSHA         string  `json:"git_sha"`
	GoVersion      string  `json:"go_version"`
	GOMAXPROCS     int     `json:"gomaxprocs"`
	Preset         string  `json:"preset"`
	N              int     `json:"n"`
	Fixture        string  `json:"fixture"`
	TotalElapsedMs int64   `json:"total_elapsed_ms"`
	URLsPerSec     float64 `json:"urls_per_sec"`
	LatencyMs      struct {
		P50 int64 `json:"p50"`
		P95 int64 `json:"p95"`
		P99 int64 `json:"p99"`
	} `json:"latency_ms"`
	MemoryBytes struct {
		StartHeap uint64 `json:"start_heap"`
		EndHeap   uint64 `json:"end_heap"`
		PeakHeap  uint64 `json:"peak_heap"`
		Growth    int64  `json:"growth"`
	} `json:"memory_bytes"`
	SuccessRate float64 `json:"success_rate"`
	Errors      int     `json:"errors"`
}

func runStress(args []string) {
	fs := flag.NewFlagSet("stress", flag.ExitOnError)
	preset := fs.String("preset", "quick", "Preset: quick|small|medium|large")
	baseline := fs.String("baseline", "", "Optional baseline JSON to gate the run (CI mode)")
	output := fs.String("output", "tests/bench/reports", "Output directory for the JSON + Markdown reports")
	fixturePath := fs.String("fixture", "testdata/index/text-npr.html", "Path to the HTML fixture served on /page")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	n, ok := presets[*preset]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown preset %q (want quick|small|medium|large)\n", *preset)
		os.Exit(2)
	}

	body, err := os.ReadFile(*fixturePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read fixture:", err)
		os.Exit(1)
	}

	srv := fixture.New().Route("GET", "/page", fixture.Body(body, "text/html; charset=utf-8"))
	defer srv.Close()
	target := srv.URL() + "/page"

	report := executeStress(n, *preset, *fixturePath, target)

	if err := os.MkdirAll(*output, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir output:", err)
		os.Exit(1)
	}
	ts := time.Now().UTC().Format("20060102-150405")
	jsonPath := filepath.Join(*output, fmt.Sprintf("stress_%s.json", ts))
	mdPath := filepath.Join(*output, fmt.Sprintf("stress_%s.md", ts))

	if err := writeJSONReport(jsonPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "write json:", err)
		os.Exit(1)
	}
	if err := atomicWrite(mdPath, renderStressMarkdown(report)); err != nil {
		fmt.Fprintln(os.Stderr, "write markdown:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", jsonPath)
	fmt.Println("wrote", mdPath)

	if *baseline != "" {
		base, err := loadBaseline(*baseline)
		if err != nil {
			fmt.Fprintln(os.Stderr, "load baseline:", err)
			os.Exit(1)
		}
		if gateErr := evaluateGate(report, base); gateErr != nil {
			fmt.Fprintln(os.Stderr, "gate failed:", gateErr)
			os.Exit(1)
		}
		fmt.Println("gate passed")
	}
}

// executeStress runs the N-iteration fetch loop and returns a populated
// StressReport. Extracted so tests can dial down N via the `quick` preset
// while still exercising the real engine + httptest path end-to-end.
func executeStress(n int, preset, fixturePath, target string) StressReport {
	lat := make([]time.Duration, n)
	heap := make([]uint64, n)
	var ms runtime.MemStats

	runtime.ReadMemStats(&ms)
	startHeap := ms.HeapInuse

	errors := 0
	t0 := time.Now()
	for i := 0; i < n; i++ {
		iterStart := time.Now()
		res := engine.FromURL(target)
		lat[i] = time.Since(iterStart)
		if res.Error != "" {
			errors++
		}
		runtime.ReadMemStats(&ms)
		heap[i] = ms.HeapInuse
	}
	total := time.Since(t0)

	runtime.ReadMemStats(&ms)
	endHeap := ms.HeapInuse

	var peak uint64
	for _, h := range heap {
		if h > peak {
			peak = h
		}
	}

	r := StressReport{
		Version:        1,
		CapturedAt:     time.Now().UTC().Format(time.RFC3339),
		GitSHA:         gitSHA(),
		GoVersion:      runtime.Version(),
		GOMAXPROCS:     runtime.GOMAXPROCS(0),
		Preset:         preset,
		N:              n,
		Fixture:        fixturePath,
		TotalElapsedMs: total.Milliseconds(),
		URLsPerSec:     float64(n) / total.Seconds(),
		SuccessRate:    1.0 - float64(errors)/float64(n),
		Errors:         errors,
	}
	r.LatencyMs.P50 = percentileMs(lat, 50)
	r.LatencyMs.P95 = percentileMs(lat, 95)
	r.LatencyMs.P99 = percentileMs(lat, 99)
	r.MemoryBytes.StartHeap = startHeap
	r.MemoryBytes.EndHeap = endHeap
	r.MemoryBytes.PeakHeap = peak
	// int64 cast so a heap that shrunk (negative growth) is representable.
	r.MemoryBytes.Growth = int64(endHeap) - int64(startHeap)
	return r
}

// percentileMs returns the requested percentile of `lat` in whole
// milliseconds (rounded down). Empty input returns 0. Uses
// nearest-rank with clamp to len-1 to avoid index overflow on p99 of a
// short series.
func percentileMs(lat []time.Duration, p int) int64 {
	if len(lat) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(lat))
	copy(sorted, lat)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx].Milliseconds()
}

// evaluateGate compares a fresh report against a committed baseline and
// returns nil on pass, an error describing every failing gate on failure.
// Lives separately from runStress so unit tests can observe the result
// without intercepting os.Exit.
//
// Tolerances:
//   - URLs/sec must be >= 0.9 * baseline (faster is fine, no upper cap).
//   - Peak heap must be <= 1.15 * baseline (smaller is fine).
func evaluateGate(got, baseline StressReport) error {
	var fails []string
	if baseline.URLsPerSec > 0 {
		minRate := 0.9 * baseline.URLsPerSec
		if got.URLsPerSec < minRate {
			fails = append(fails, fmt.Sprintf(
				"urls_per_sec %.2f < %.2f (10%% below baseline %.2f)",
				got.URLsPerSec, minRate, baseline.URLsPerSec))
		}
	}
	if baseline.MemoryBytes.PeakHeap > 0 {
		maxPeak := uint64(float64(baseline.MemoryBytes.PeakHeap) * 1.15)
		if got.MemoryBytes.PeakHeap > maxPeak {
			fails = append(fails, fmt.Sprintf(
				"peak_heap %d > %d (15%% above baseline %d)",
				got.MemoryBytes.PeakHeap, maxPeak, baseline.MemoryBytes.PeakHeap))
		}
	}
	if len(fails) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(fails, "; "))
}

func loadBaseline(path string) (StressReport, error) {
	var r StressReport
	raw, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return r, fmt.Errorf("parse %s: %w", path, err)
	}
	return r, nil
}

func writeJSONReport(path string, r StressReport) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, string(raw)+"\n")
}

func renderStressMarkdown(r StressReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# SeaPortal Stress Report\n\n")
	fmt.Fprintf(&b, "- Captured: %s\n", r.CapturedAt)
	fmt.Fprintf(&b, "- Git SHA: `%s`\n", r.GitSHA)
	fmt.Fprintf(&b, "- Go: %s, GOMAXPROCS=%d\n", r.GoVersion, r.GOMAXPROCS)
	fmt.Fprintf(&b, "- Preset: `%s` (N=%d)\n", r.Preset, r.N)
	fmt.Fprintf(&b, "- Fixture: `%s`\n\n", r.Fixture)

	fmt.Fprintln(&b, "## Throughput")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "| Metric | Value |")
	fmt.Fprintln(&b, "|---|---|")
	fmt.Fprintf(&b, "| Total elapsed | %d ms |\n", r.TotalElapsedMs)
	fmt.Fprintf(&b, "| URLs / sec | %.2f |\n", r.URLsPerSec)
	fmt.Fprintf(&b, "| Success rate | %.4f |\n", r.SuccessRate)
	fmt.Fprintf(&b, "| Errors | %d |\n", r.Errors)
	fmt.Fprintln(&b, "")

	fmt.Fprintln(&b, "## Latency (ms)")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "| p50 | p95 | p99 |")
	fmt.Fprintln(&b, "|---|---|---|")
	fmt.Fprintf(&b, "| %d | %d | %d |\n\n", r.LatencyMs.P50, r.LatencyMs.P95, r.LatencyMs.P99)

	fmt.Fprintln(&b, "## Memory (HeapInuse bytes)")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "| Start | End | Peak | Growth |")
	fmt.Fprintln(&b, "|---|---|---|---|")
	fmt.Fprintf(&b, "| %d | %d | %d | %d |\n",
		r.MemoryBytes.StartHeap, r.MemoryBytes.EndHeap,
		r.MemoryBytes.PeakHeap, r.MemoryBytes.Growth)
	return b.String()
}

// gitSHA returns the short git SHA of HEAD, or "unknown" outside a git tree.
// Best-effort: stress reports without a SHA are still useful for ad-hoc runs.
func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
