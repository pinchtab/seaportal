package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTokens_TinyCorpusRoundTrip wires a 2-entry tempdir corpus, drives
// runTokens through its public CLI seam, and asserts the JSON report
// parses and carries 2 fixtures × 4 modes = 8 mode-rows. Light on
// extraction expectations — only the wiring is under test.
func TestTokens_TinyCorpusRoundTrip(t *testing.T) {
	dir := t.TempDir()

	articleHTML := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Sample Article</title></head><body>
<h1>Sample Article</h1>
<p>This is the first paragraph, with a <a href="https://example.com/a">link</a> for the link-retention modes to exercise.</p>
<p>Second paragraph has another <a href="https://example.com/b">link</a> plus enough body text to clear thresholds.</p>
<p>Third paragraph keeps the paragraph count comfortable for the extractor to surface content.</p>
</body></html>`

	plainHTML := `<!DOCTYPE html><html lang="en"><head><title>Plain</title></head><body>
<h1>Plain page</h1>
<p>No links here at all, just words and more words so the extractor has something meaningful to work with on this small page.</p>
<p>Another paragraph for shape so the extraction does not bail out on a sparse body.</p>
</body></html>`

	articlePath := filepath.Join(dir, "article.html")
	plainPath := filepath.Join(dir, "plain.html")
	mustWrite(t, articlePath, articleHTML)
	mustWrite(t, plainPath, plainHTML)

	corpusYAML := "- path: " + articlePath + "\n  expect_class: static\n" +
		"- path: " + plainPath + "\n  expect_class: static\n"
	corpusPath := filepath.Join(dir, "corpus.yaml")
	mustWrite(t, corpusPath, corpusYAML)

	outDir := filepath.Join(dir, "out")
	runTokens([]string{"--corpus", corpusPath, "--output", outDir})

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	var jsonFile string
	gotMD := false
	for _, e := range entries {
		name := e.Name()
		switch {
		case strings.HasSuffix(name, ".json"):
			jsonFile = filepath.Join(outDir, name)
		case strings.HasSuffix(name, ".md"):
			gotMD = true
		}
	}
	if jsonFile == "" || !gotMD {
		t.Fatalf("expected json+md in %s, got %v", outDir, entries)
	}

	raw, err := os.ReadFile(jsonFile)
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var report TokensReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if report.Version != 1 {
		t.Errorf("version = %d, want 1", report.Version)
	}
	if report.TotalFixtures != 2 {
		t.Fatalf("total_fixtures = %d, want 2", report.TotalFixtures)
	}
	if len(report.PerFixture) != 2 {
		t.Fatalf("per_fixture len = %d, want 2", len(report.PerFixture))
	}
	// 2 fixtures × 4 modes = 8 mode-rows.
	totalModeRows := 0
	for _, row := range report.PerFixture {
		if len(row.Modes) != 4 {
			t.Errorf("fixture %q: modes len = %d, want 4", row.Path, len(row.Modes))
		}
		totalModeRows += len(row.Modes)
		for _, name := range []string{"all", "none", "text", "footer"} {
			if _, ok := row.Modes[name]; !ok {
				t.Errorf("fixture %q: missing mode %q", row.Path, name)
			}
		}
	}
	if totalModeRows != 8 {
		t.Fatalf("total mode-rows = %d, want 8", totalModeRows)
	}
	for _, name := range []string{"all", "none", "text", "footer"} {
		if _, ok := report.PerMode[name]; !ok {
			t.Errorf("per_mode missing %q", name)
		}
	}
}

// TestApproxTokenCount_Deterministic guards the "no map iteration, no
// randomness" contract. Three consecutive calls with the same input must
// return identical counts — otherwise CI gates on top of this would be
// flaky for the wrong reasons.
func TestApproxTokenCount_Deterministic(t *testing.T) {
	input := "Hello, world! This is a (small) test... with punctuation; and links: https://example.com/x."
	first := approxTokenCount(input)
	for i := 0; i < 2; i++ {
		got := approxTokenCount(input)
		if got != first {
			t.Fatalf("approxTokenCount drift: call %d returned %d, first returned %d", i+2, got, first)
		}
	}
	// Empty string sanity.
	if approxTokenCount("") != 0 {
		t.Errorf("approxTokenCount(\"\") = %d, want 0", approxTokenCount(""))
	}
}

// TestApproxTokenCount_RoughlyMonotonic confirms that appending content
// never lowers the count — basic sanity check that protects against a
// regression where the cluster counter accidentally subtracts.
func TestApproxTokenCount_RoughlyMonotonic(t *testing.T) {
	base := "alpha beta gamma"
	more := base + " delta epsilon zeta"
	even := more + " eta theta iota kappa lambda mu nu xi omicron"
	a, b, c := approxTokenCount(base), approxTokenCount(more), approxTokenCount(even)
	if a > b || b > c {
		t.Fatalf("not monotonic: %d, %d, %d", a, b, c)
	}
	if a == 0 {
		t.Fatalf("base count = 0, expected non-zero")
	}
}
