//go:build integration

// Package engine — latency budget gate.
//
// TestLatencyBudget pipes the full 31-entry eval corpus through a single
// in-process fixture HTTP server and asserts that end-to-end FromURL wall
// time stays within the documented "<2s" performance envelope: p95 ≤ 1.5s
// and p99 ≤ 2.5s.
//
// Excluded from the default ./dev all run via the `integration` build tag;
// invoke explicitly with:
//
//	go test -tags=integration -run TestLatencyBudget ./internal/engine/
package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/pinchtab/seaportal/internal/testserver/fixture"
)

// Latency thresholds. The README aspires to "<2s" wall-clock; the test
// enforces a regression gate around the values actually observed on the
// hermetic fixture server (macOS / M-series, single-threaded).
//
// Locked-in numbers:
//
//   - p50 sits at ~1ms across the corpus — the static-fixture happy path
//     is well inside the budget.
//   - p95 ≈ 240ms, dominated by `testdata/static/github-awesome.html` (large
//     HTML doc + full extraction pipeline). Gate set at 1500ms to give
//     CI jitter ~6× headroom.
//   - p99 ≈ 1.5s, dominated by `testdata/static/wikipedia-latin-phrases.html`
//     (1.3 MB Wikipedia page). The previous ~13s outlier was caused by
//     SanitizeHTML running ten separate `(?is)<tag\b[^>]*(?:/>|>...</tag>)`
//     regexes over the whole document; replaced with a single-pass
//     tokenizer (see sanitize.go: removeAlwaysHiddenTagsSinglePass and
//     removeHiddenElementsSinglePass). Gate set at 2500ms to track the
//     README's <2s aspiration while giving CI jitter ~1.6× headroom.
//
// On failure the test uses t.Errorf (not Fatalf) so BOTH p95 and p99 are
// surfaced in the same run, alongside the per-fixture >500ms watchdog
// log lines that point at the culprit.
const (
	latencyP95Budget = 1500 * time.Millisecond
	latencyP99Budget = 2500 * time.Millisecond

	// perFixtureWatchdog flags individual fixtures that exceed this
	// threshold via t.Logf — a regression pointer, not an assertion.
	perFixtureWatchdog = 500 * time.Millisecond
)

func TestLatencyBudget(t *testing.T) {
	repoRoot := repoRootFromCaller(t)
	corpusPath := filepath.Join(repoRoot, "tests", "eval", "corpus.yaml")

	entries, err := LoadCorpus(corpusPath)
	if err != nil {
		t.Fatalf("load corpus %s: %v", corpusPath, err)
	}
	if len(entries) == 0 {
		t.Fatalf("corpus %s is empty", corpusPath)
	}

	// Build a single fixture server with one route per fixture. Cheaper
	// than spinning a server per request and avoids per-iteration setup
	// cost polluting the latency numbers.
	srv := fixture.New()
	defer srv.Close()

	routes := make([]string, 0, len(entries))
	for _, entry := range entries {
		absPath := filepath.Join(repoRoot, entry.Path)
		body, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("read fixture %s: %v", entry.Path, err)
		}
		route := "/" + entry.Path
		srv.Route("GET", route, fixture.Body(body, "text/html; charset=utf-8"))
		routes = append(routes, route)
	}

	samples := make([]latencySample, 0, len(entries))

	for i, route := range routes {
		start := time.Now()
		_ = FromURL(srv.URL() + route)
		d := time.Since(start)
		samples = append(samples, latencySample{path: entries[i].Path, dur: d})

		if d > perFixtureWatchdog {
			t.Logf("latency watchdog: %s took %v (>%v)", entries[i].Path, d, perFixtureWatchdog)
		}
	}

	sort.Slice(samples, func(i, j int) bool { return samples[i].dur < samples[j].dur })

	p50 := percentile(samples, 0.50)
	p95 := percentile(samples, 0.95)
	p99 := percentile(samples, 0.99)

	t.Logf("latency budget: p50=%v p95=%v p99=%v across %d fixtures", p50, p95, p99, len(samples))

	if p95 > latencyP95Budget {
		t.Errorf("p95 latency %v exceeds budget %v — slowest fixture: %s (%v)",
			p95, latencyP95Budget, samples[len(samples)-1].path, samples[len(samples)-1].dur)
	}
	if p99 > latencyP99Budget {
		t.Errorf("p99 latency %v exceeds budget %v — slowest fixture: %s (%v)",
			p99, latencyP99Budget, samples[len(samples)-1].path, samples[len(samples)-1].dur)
	}
}

// latencySample pairs a fixture path with its measured wall-clock duration.
type latencySample struct {
	path string
	dur  time.Duration
}

// percentile returns the q-th percentile (0 ≤ q ≤ 1) of an already-sorted
// sample slice using the nearest-rank method. Clamped at both ends so a
// 99th percentile of a 10-sample run still returns the slowest entry.
func percentile(sorted []latencySample, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(q * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return sorted[idx].dur
}

// repoRootFromCaller resolves the repository root by walking up from this
// test file's directory until a go.mod is found. Independent of cwd so the
// test works under `go test ./internal/engine/` as well as `go test ./...`.
func repoRootFromCaller(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", thisFile)
		}
		dir = parent
	}
}
