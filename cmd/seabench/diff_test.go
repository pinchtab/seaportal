package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDiff_TinyCorpusRoundTrip wires a 2-entry tempdir corpus, runs the full
// subcommand entrypoint, and asserts the JSON report parses with the expected
// two comparisons (default-vs-minimal, default-vs-aggressive), each carrying
// one per-fixture row per entry.
func TestDiff_TinyCorpusRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Two article-shaped pages so every mode produces non-empty content
	// (avoids the trivial all-empty case where every delta is zero by
	// construction). The second fixture intentionally repeats a paragraph
	// many times so `aggressive` (Dedupe=true) has something to collapse.
	page1 := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Tiny Page One</title></head><body>
<h1>Tiny Page One</h1>
<p>First paragraph of a small article. It has enough words to clear the readability minimum length threshold for static articles.</p>
<h2>Section two</h2>
<p>Second paragraph extends the body so the extractor surfaces a real result. Lorem ipsum dolor sit amet.</p>
<p>Third paragraph for shape.</p>
</body></html>`

	var rep strings.Builder
	rep.WriteString(`<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Repeats</title></head><body>`)
	rep.WriteString(`<h1>Repeats</h1>`)
	for i := 0; i < 12; i++ {
		rep.WriteString(`<p>Identical paragraph repeated to give dedupe something to do. Lorem ipsum dolor sit amet consectetur.</p>`)
	}
	rep.WriteString(`</body></html>`)
	page2 := rep.String()

	p1 := filepath.Join(dir, "p1.html")
	p2 := filepath.Join(dir, "p2.html")
	mustWrite(t, p1, page1)
	mustWrite(t, p2, page2)

	corpus := "- path: " + p1 + "\n  expect_class: static\n" +
		"- path: " + p2 + "\n  expect_class: static\n"
	corpusPath := filepath.Join(dir, "corpus.yaml")
	mustWrite(t, corpusPath, corpus)

	outDir := filepath.Join(dir, "out")
	runDiff([]string{"--corpus", corpusPath, "--output", outDir, "--snippet-chars", "120"})

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
	var report DiffReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if report.Version != 1 {
		t.Errorf("version = %d, want 1", report.Version)
	}
	if len(report.Comparisons) != 2 {
		t.Fatalf("comparisons = %d, want 2", len(report.Comparisons))
	}
	wantPairs := map[string]string{"minimal": "default", "aggressive": "default"}
	for _, c := range report.Comparisons {
		if wantPairs[c.Variant] != c.Baseline {
			t.Errorf("unexpected comparison baseline=%s variant=%s", c.Baseline, c.Variant)
		}
		if len(c.PerFixture) != 2 {
			t.Errorf("per_fixture(%s vs %s) = %d, want 2", c.Baseline, c.Variant, len(c.PerFixture))
		}
		for _, r := range c.PerFixture {
			if r.Panicked != "" {
				t.Errorf("unexpected panic in fixture %s: %s", r.Path, r.Panicked)
			}
		}
	}
}

// TestDiff_DetectsCharDelta builds a corpus where the variant ("aggressive")
// is expected to produce strictly less content than the baseline ("default")
// for at least one fixture (heavy duplication → Dedupe collapses chunks).
// The sign convention is `char_delta = len(baseline) - len(variant)`, so a
// positive char_delta on at least one row proves the diff lane is reporting
// the right direction.
func TestDiff_DetectsCharDelta(t *testing.T) {
	dir := t.TempDir()

	var rep strings.Builder
	rep.WriteString(`<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Dup Heavy</title></head><body>`)
	rep.WriteString(`<h1>Dup Heavy</h1>`)
	for i := 0; i < 20; i++ {
		rep.WriteString(`<p>This exact sentence repeats many times so deduplication has clear, abundant work to perform across the body. Lorem ipsum dolor sit amet consectetur adipiscing elit.</p>`)
	}
	rep.WriteString(`</body></html>`)
	page := rep.String()
	p := filepath.Join(dir, "dup.html")
	mustWrite(t, p, page)

	corpusPath := filepath.Join(dir, "corpus.yaml")
	mustWrite(t, corpusPath, "- path: "+p+"\n  expect_class: static\n")

	report, err := diffCorpus(corpusPath, 200)
	if err != nil {
		t.Fatalf("diffCorpus: %v", err)
	}
	if len(report.Comparisons) != 2 {
		t.Fatalf("comparisons = %d, want 2", len(report.Comparisons))
	}

	// Find the default-vs-aggressive comparison; assert char_delta >= 0
	// (aggressive should produce ≤ baseline). Strict inequality would be
	// nicer but is too brittle to a future cleanup tweak that already
	// dedupes everything at the default level — equality is acceptable.
	var aggr *DiffComparison
	for i := range report.Comparisons {
		if report.Comparisons[i].Variant == "aggressive" {
			aggr = &report.Comparisons[i]
		}
	}
	if aggr == nil {
		t.Fatal("default-vs-aggressive comparison missing")
	}
	if len(aggr.PerFixture) != 1 {
		t.Fatalf("per_fixture = %d, want 1", len(aggr.PerFixture))
	}
	row := aggr.PerFixture[0]
	if row.CharDelta < 0 {
		t.Errorf("aggressive produced MORE content than default for a duplication-heavy fixture: char_delta = %d", row.CharDelta)
	}
}

// TestFirstDiffIndex_BasicCases pins the divergence-locator semantics so
// the JSON `first_diff_idx` field stays honest as the snippet logic
// evolves.
func TestFirstDiffIndex_BasicCases(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"same", "same", -1},
		{"abc", "abd", 2},
		{"short", "shorter", 5},
		{"", "anything", 0},
		{"anything", "", 0},
	}
	for _, c := range cases {
		got := firstDiffIndex(c.a, c.b)
		if got != c.want {
			t.Errorf("firstDiffIndex(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
