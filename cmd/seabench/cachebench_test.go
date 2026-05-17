package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/seaportal/internal/engine/leakcheck"
)

// TestCacheBench_QuickRoundTrip drives the CLI flow end-to-end with a small
// N against an in-process fixture server. Asserts: JSON + Markdown reports
// land on disk, all 3 modes are present with sensible numbers, hit-rate
// invariants hold (off == 0, ttl-24h > 0, swr-10m > 0).
func TestCacheBench_QuickRoundTrip(t *testing.T) {
	leakcheck.CheckLeak(t)
	reportDir := filepath.Join(t.TempDir(), "reports")

	runCacheBench([]string{
		"--n", "20",
		"--hot", "3",
		"--cold", "10",
		"--output", reportDir,
	})

	entries, err := os.ReadDir(reportDir)
	if err != nil {
		t.Fatal(err)
	}
	var jsonPath, mdPath string
	for _, e := range entries {
		switch {
		case strings.HasPrefix(e.Name(), "cachebench_") && strings.HasSuffix(e.Name(), ".json"):
			jsonPath = filepath.Join(reportDir, e.Name())
		case strings.HasPrefix(e.Name(), "cachebench_") && strings.HasSuffix(e.Name(), ".md"):
			mdPath = filepath.Join(reportDir, e.Name())
		}
	}
	if jsonPath == "" {
		t.Fatalf("no cachebench_*.json in %s; got %v", reportDir, entries)
	}
	if mdPath == "" {
		t.Fatalf("no cachebench_*.md in %s; got %v", reportDir, entries)
	}

	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var r CacheBenchReport
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("parse json: %v\n%s", err, raw)
	}
	if r.Version != 1 {
		t.Errorf("version = %d, want 1", r.Version)
	}
	if r.N != 20 {
		t.Errorf("N = %d, want 20", r.N)
	}
	for _, mode := range cacheModeOrder {
		s, ok := r.PerMode[mode]
		if !ok {
			t.Errorf("missing mode %q in per_mode", mode)
			continue
		}
		if s.Requests != 20 {
			t.Errorf("mode %q: requests = %d, want 20", mode, s.Requests)
		}
	}
	if r.PerMode["off"].HitRate != 0 {
		t.Errorf("mode off: hit_rate = %.4f, want 0", r.PerMode["off"].HitRate)
	}
	if r.PerMode["ttl-24h"].HitRate <= 0 {
		t.Errorf("mode ttl-24h: hit_rate = %.4f, want > 0", r.PerMode["ttl-24h"].HitRate)
	}
	if r.PerMode["swr-10m"].HitRate <= 0 {
		t.Errorf("mode swr-10m: hit_rate = %.4f, want > 0", r.PerMode["swr-10m"].HitRate)
	}

	md, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# SeaPortal Cache Bench Report", "## Per-mode", "off", "ttl-24h", "swr-10m"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("markdown report missing %q", want)
		}
	}
}

// TestCacheBench_DeterministicSampling asserts that the URL sequence
// generator returns the identical sequence for two calls with the same
// seed and hot-ratio — the cornerstone of cross-mode comparability.
func TestCacheBench_DeterministicSampling(t *testing.T) {
	hot := []string{"h0", "h1", "h2"}
	cold := []string{"c0", "c1", "c2", "c3", "c4"}
	a := generateURLSequence(100, 0.8, hot, cold, 42)
	b := generateURLSequence(100, 0.8, hot, cold, 42)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sampling not deterministic at i=%d: %q vs %q", i, a[i], b[i])
		}
	}
	// Sanity: ~80% of draws should be hot.
	hotHits := 0
	hotSet := map[string]bool{"h0": true, "h1": true, "h2": true}
	for _, u := range a {
		if hotSet[u] {
			hotHits++
		}
	}
	if hotHits < 70 || hotHits > 90 {
		t.Errorf("expected ~80 hot draws out of 100, got %d", hotHits)
	}
}
