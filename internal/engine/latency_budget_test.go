//go:build integration

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

const (
	latencyP95Budget = 1500 * time.Millisecond
	latencyP99Budget = 2500 * time.Millisecond

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

	srv := fixture.New()
	defer srv.Close()

	routes := make([]string, 0, len(entries))
	registered := make(map[string]bool, len(entries))
	for _, entry := range entries {
		route := "/" + entry.Path
		if !registered[route] {
			absPath := filepath.Join(repoRoot, entry.Path)
			body, err := os.ReadFile(absPath)
			if err != nil {
				t.Fatalf("read fixture %s: %v", entry.Path, err)
			}
			srv.Route("GET", route, fixture.Body(body, "text/html; charset=utf-8"))
			registered[route] = true
		}
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

type latencySample struct {
	path string
	dur  time.Duration
}

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
