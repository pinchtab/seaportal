//go:build allocs && !race

package engine

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
)

// TestAllocationBudgets re-runs the engine's hot-path benchmarks via
// testing.Benchmark(...) and compares B/op and allocs/op against the
// committed baseline at tests/bench/profiles/allocs_baseline.json.
//
// A drift greater than ±tolerance_pct (default 15%) in either direction
// fails the test. The gate is opt-in via the `allocs` build tag so the
// default `go test ./...` and `./dev all` flows stay fast.
//
// Regenerate the baseline with: scripts/regen-allocs-baseline.sh
//
// The race detector inflates allocation counts dramatically; the
// `&& !race` half of the build tag prevents accidental runs under
// `-race` from producing misleading failures.

type allocsBaselineEntry struct {
	BPerOp      int64 `json:"b_per_op"`
	AllocsPerOp int64 `json:"allocs_per_op"`
}

type allocsBaseline struct {
	Version      int                            `json:"version"`
	CapturedAt   string                         `json:"captured_at"`
	GoVersion    string                         `json:"go_version"`
	GoMaxProcs   int                            `json:"gomaxprocs"`
	Benchtime    string                         `json:"benchtime"`
	TolerancePct float64                        `json:"tolerance_pct"`
	Benchmarks   map[string]allocsBaselineEntry `json:"benchmarks"`
}

// registeredAllocBenches maps baseline benchmark name -> bench function.
// When a new benchmark is added (or removed), update this map AND
// regenerate the baseline. The test fails loudly on either direction of
// drift between this map and the JSON.
var registeredAllocBenches = map[string]func(*testing.B){
	"BenchmarkExtract_Local":                  BenchmarkExtract_Local,
	"BenchmarkFromHTML_WikipediaLatinPhrases": BenchmarkFromHTML_WikipediaLatinPhrases,
	"BenchmarkPreprocess_Wikipedia":           BenchmarkPreprocess_Wikipedia,
	"BenchmarkSanitize_Wikipedia":             BenchmarkSanitize_Wikipedia,
	"BenchmarkCleanup_Wikipedia":              BenchmarkCleanup_Wikipedia,
	"BenchmarkDedupe_Wikipedia":               BenchmarkDedupe_Wikipedia,
	"BenchmarkSnapshot_Wikipedia":             BenchmarkSnapshot_Wikipedia,
}

func TestAllocationBudgets(t *testing.T) {
	const baselinePath = "../../tests/bench/profiles/allocs_baseline.json"

	raw, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline %s: %v", baselinePath, err)
	}
	var baseline allocsBaseline
	if err := json.Unmarshal(raw, &baseline); err != nil {
		t.Fatalf("parse baseline %s: %v", baselinePath, err)
	}
	if baseline.Version != 1 {
		t.Fatalf("unsupported baseline version %d (want 1)", baseline.Version)
	}
	if len(baseline.Benchmarks) == 0 {
		t.Fatalf("baseline has no benchmark entries")
	}

	tolerance := baseline.TolerancePct / 100.0
	if tolerance <= 0 {
		t.Fatalf("baseline tolerance_pct must be > 0 (got %v)", baseline.TolerancePct)
	}

	// Detect drift between baseline JSON and registered map in both directions.
	for name := range baseline.Benchmarks {
		if _, ok := registeredAllocBenches[name]; !ok {
			t.Errorf("baseline lists %q but no Go binding is registered in registeredAllocBenches", name)
		}
	}
	for name := range registeredAllocBenches {
		if _, ok := baseline.Benchmarks[name]; !ok {
			t.Errorf("registeredAllocBenches has %q but baseline JSON has no entry — run scripts/regen-allocs-baseline.sh", name)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	// Iterate in sorted order for deterministic subtest output.
	names := make([]string, 0, len(baseline.Benchmarks))
	for name := range baseline.Benchmarks {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		want := baseline.Benchmarks[name]
		fn := registeredAllocBenches[name]
		t.Run(name, func(t *testing.T) {
			res := testing.Benchmark(fn)
			gotB := res.AllocedBytesPerOp()
			gotAllocs := res.AllocsPerOp()

			if !withinTolerance(gotB, want.BPerOp, tolerance) {
				t.Errorf("B/op = %d; baseline %d (tolerance ±%.0f%%, drift %.1f%%)",
					gotB, want.BPerOp, baseline.TolerancePct, pctDrift(gotB, want.BPerOp))
			}
			if !withinTolerance(gotAllocs, want.AllocsPerOp, tolerance) {
				t.Errorf("allocs/op = %d; baseline %d (tolerance ±%.0f%%, drift %.1f%%)",
					gotAllocs, want.AllocsPerOp, baseline.TolerancePct, pctDrift(gotAllocs, want.AllocsPerOp))
			}
		})
	}
}

// withinTolerance reports whether got is within ±tolerance (fractional)
// of want. A want of 0 is matched only by got of 0.
func withinTolerance(got, want int64, tolerance float64) bool {
	if want == 0 {
		return got == 0
	}
	drift := math.Abs(float64(got-want)) / math.Abs(float64(want))
	return drift <= tolerance
}

func pctDrift(got, want int64) float64 {
	if want == 0 {
		if got == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return (float64(got-want) / math.Abs(float64(want))) * 100.0
}
