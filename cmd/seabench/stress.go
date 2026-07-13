package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/pinchtab/seaportal"
	"github.com/pinchtab/seaportal/internal/testserver/fixture"
)

var presets = map[string]int{
	"quick":  50,
	"small":  200,
	"medium": 500,
	"large":  2000,
}

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

	_, mdPath, err := emitReports(*output, "stress", report, renderStressMarkdown(report))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("stress: %.0f urls/s, %.0f%% success, p50=%dms (n=%d, %s). See %s\n",
		report.URLsPerSec, report.SuccessRate*100, report.LatencyMs.P50, report.N, report.Preset, mdPath)

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
		res := seaportal.FromURL(target)
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
	r.LatencyMs.P50 = percentile(lat, 0.50).Milliseconds()
	r.LatencyMs.P95 = percentile(lat, 0.95).Milliseconds()
	r.LatencyMs.P99 = percentile(lat, 0.99).Milliseconds()
	r.MemoryBytes.StartHeap = startHeap
	r.MemoryBytes.EndHeap = endHeap
	r.MemoryBytes.PeakHeap = peak
	r.MemoryBytes.Growth = int64(endHeap) - int64(startHeap)
	return r
}

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

func renderStressMarkdown(r StressReport) string {
	var b strings.Builder
	reportHeader(&b, "SeaPortal Stress Report",
		"Captured", r.CapturedAt,
		"Git SHA", "`"+r.GitSHA+"`")
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

func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
