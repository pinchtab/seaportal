package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/seaportal/internal/engine/leakcheck"
)

// stressFixtureHTML is a ~6KB HTML blob that's representative of a real
// article page without dragging in any external testdata. Kept inline so the
// test has no path-resolution dependency on the repo root — `go test
// ./cmd/seabench/...` runs from cmd/seabench/ where relative testdata paths
// won't resolve.
const stressFixtureBody = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. "

func writeStressFixture(t *testing.T, dir string) string {
	t.Helper()
	// Pad the body to ~6KB so the engine has realistic work to do per request.
	var sb strings.Builder
	sb.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8">`)
	sb.WriteString(`<title>Stress Article</title>`)
	sb.WriteString(`<meta name="description" content="A representative article for stress testing.">`)
	sb.WriteString(`</head><body><article><h1>Stress Article</h1>`)
	for i := 0; i < 30; i++ {
		sb.WriteString("<p>")
		sb.WriteString(stressFixtureBody)
		sb.WriteString("</p>")
	}
	sb.WriteString(`</article></body></html>`)

	path := filepath.Join(dir, "stress-fixture.html")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestStress_QuickPresetRoundTrip exercises the full runStress flow against
// a tempdir-hosted fixture: 50 fetches against the in-process server, JSON +
// Markdown reports written, JSON parses, N matches preset, success > 99%.
func TestStress_QuickPresetRoundTrip(t *testing.T) {
	leakcheck.CheckLeak(t)
	dir := t.TempDir()
	fixture := writeStressFixture(t, dir)
	reportDir := filepath.Join(dir, "reports")

	runStress([]string{
		"--preset", "quick",
		"--output", reportDir,
		"--fixture", fixture,
	})

	entries, err := os.ReadDir(reportDir)
	if err != nil {
		t.Fatal(err)
	}
	var jsonPath, mdPath string
	for _, e := range entries {
		switch {
		case strings.HasPrefix(e.Name(), "stress_") && strings.HasSuffix(e.Name(), ".json"):
			jsonPath = filepath.Join(reportDir, e.Name())
		case strings.HasPrefix(e.Name(), "stress_") && strings.HasSuffix(e.Name(), ".md"):
			mdPath = filepath.Join(reportDir, e.Name())
		}
	}
	if jsonPath == "" {
		t.Fatalf("no stress_*.json in %s; got: %v", reportDir, entries)
	}
	if mdPath == "" {
		t.Fatalf("no stress_*.md in %s; got: %v", reportDir, entries)
	}

	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var r StressReport
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("parse json report: %v\n%s", err, raw)
	}
	if r.Version != 1 {
		t.Errorf("version = %d, want 1", r.Version)
	}
	if r.N != 50 {
		t.Errorf("N = %d, want 50", r.N)
	}
	if r.Preset != "quick" {
		t.Errorf("preset = %q, want quick", r.Preset)
	}
	if r.SuccessRate <= 0.99 {
		t.Errorf("success_rate = %.4f, want > 0.99 (errors=%d)", r.SuccessRate, r.Errors)
	}
	if r.URLsPerSec <= 0 {
		t.Errorf("urls_per_sec = %.2f, want > 0", r.URLsPerSec)
	}
	if r.MemoryBytes.PeakHeap == 0 {
		t.Errorf("peak_heap = 0, want > 0")
	}

	md, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# SeaPortal Stress Report", "## Throughput", "## Latency", "## Memory"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("markdown report missing %q", want)
		}
	}
}

// TestStress_BaselineGate_Passes feeds the gate function a baseline the
// current run easily beats (1 URL/sec, 1 GiB peak). Asserts nil error.
func TestStress_BaselineGate_Passes(t *testing.T) {
	got := StressReport{URLsPerSec: 100}
	got.MemoryBytes.PeakHeap = 10 * 1024 * 1024 // 10 MiB
	baseline := StressReport{URLsPerSec: 1}
	baseline.MemoryBytes.PeakHeap = 1024 * 1024 * 1024 // 1 GiB
	if err := evaluateGate(got, baseline); err != nil {
		t.Fatalf("gate should pass, got: %v", err)
	}
}

// TestStress_BaselineGate_Fails feeds the gate a tiny baseline peak heap
// (1KB) — the actual run is guaranteed to blow past it. Asserts a non-nil
// error that mentions peak_heap.
func TestStress_BaselineGate_Fails(t *testing.T) {
	got := StressReport{URLsPerSec: 100}
	got.MemoryBytes.PeakHeap = 10 * 1024 * 1024 // 10 MiB observed
	baseline := StressReport{URLsPerSec: 100}
	baseline.MemoryBytes.PeakHeap = 1024 // 1 KiB baseline → 1.15 KiB ceiling
	err := evaluateGate(got, baseline)
	if err == nil {
		t.Fatal("gate should fail when peak_heap exceeds 1.15x baseline, got nil")
	}
	if !strings.Contains(err.Error(), "peak_heap") {
		t.Errorf("gate error should mention peak_heap, got: %v", err)
	}

	// Also: URLs/sec gate fires when we're slower than 0.9x baseline.
	got2 := StressReport{URLsPerSec: 10}
	baseline2 := StressReport{URLsPerSec: 100}
	err2 := evaluateGate(got2, baseline2)
	if err2 == nil || !strings.Contains(err2.Error(), "urls_per_sec") {
		t.Errorf("urls_per_sec gate should fail when slower, got: %v", err2)
	}
}
