package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// writeReportJSON marshals v with two-space indentation and atomically
// writes it (plus a trailing newline) to path.
func writeReportJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, string(raw)+"\n")
}

// emitReports writes the shared JSON + Markdown report pair for a
// subcommand — <outputDir>/<cmdName>_<UTC timestamp>.{json,md} — creating
// the output directory first and printing the standard "wrote <path>"
// lines. Both paths are returned so callers can derive siblings (e.g.
// classify's CSV) or reference the Markdown in their headline line.
func emitReports(outputDir, cmdName string, report any, markdown string) (jsonPath, mdPath string, err error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", "", fmt.Errorf("mkdir output: %w", err)
	}
	ts := time.Now().UTC().Format("20060102-150405")
	jsonPath = filepath.Join(outputDir, fmt.Sprintf("%s_%s.json", cmdName, ts))
	mdPath = filepath.Join(outputDir, fmt.Sprintf("%s_%s.md", cmdName, ts))
	if err := writeReportJSON(jsonPath, report); err != nil {
		return "", "", fmt.Errorf("write json: %w", err)
	}
	if err := atomicWrite(mdPath, markdown); err != nil {
		return "", "", fmt.Errorf("write markdown: %w", err)
	}
	fmt.Println("wrote", jsonPath)
	fmt.Println("wrote", mdPath)
	return jsonPath, mdPath, nil
}

// reportHeader writes the shared Markdown preamble — "# <title>", a blank
// line, then one "- key: value" bullet per pair. Callers append any
// lane-specific bullets plus the closing blank line themselves.
func reportHeader(b *strings.Builder, title string, kv ...string) {
	fmt.Fprintf(b, "# %s\n\n", title)
	for i := 0; i+1 < len(kv); i += 2 {
		fmt.Fprintf(b, "- %s: %s\n", kv[i], kv[i+1])
	}
}

// atomicWrite writes to <path>.tmp then renames into place so a partial
// run can't corrupt a previously-written report.
func atomicWrite(path string, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// renderReport produces the Markdown report. Two sections:
//   - Headline table: micro-averaged Precision / Recall / F1 per extractor,
//     plus the mean time ratio against strip-tags and a skipped count.
//   - Per-fixture detail: one row per (fixture, extractor) with raw
//     TP/FP/FN, P/R/F1, and absolute median time in microseconds.
//
// Absolute milliseconds appear only in the detail section; the headline
// table is kept machine-independent (ratios only) so report diffs across
// hardware are meaningful.
func renderReport(corpusPath string, extractors []extractor, agg []aggregate, perFixture map[string][]scoreCard) string {
	var b strings.Builder

	reportHeader(&b, "SeaPortal Eval Bake-off",
		"Generated", time.Now().UTC().Format(time.RFC3339),
		"Corpus", "`"+corpusPath+"`")
	fmt.Fprintf(&b, "- Extractors: %d\n", len(extractors))
	fmt.Fprintf(&b, "- Fixtures: %d\n\n", len(perFixture))

	fmt.Fprintln(&b, "## Headline")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "Micro-averaged across all signal-bearing fixtures. Time ratio is mean(ratio_per_fixture) where ratio_per_fixture = extractor_median_ns / strip-tags_median_ns.")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "| Extractor | Precision | Recall | F1 | Time ratio | Skipped (n/total) |")
	fmt.Fprintln(&b, "|---|---|---|---|---|---|")
	for _, a := range agg {
		fmt.Fprintf(&b, "| %s | %.2f | %.2f | %.2f | %.2fx | %d/%d |\n",
			a.Extractor, a.Precision, a.Recall, a.F1, a.AvgTimeRatio, a.Skipped, a.Total)
	}
	fmt.Fprintln(&b, "")

	fmt.Fprintln(&b, "## Per-fixture detail")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "| Fixture | Extractor | TP | FP | FN | P | R | F1 | Time (us) | Notes |")
	fmt.Fprintln(&b, "|---|---|---|---|---|---|---|---|---|---|")

	fixtures := make([]string, 0, len(perFixture))
	for k := range perFixture {
		fixtures = append(fixtures, k)
	}
	sort.Strings(fixtures)

	for _, fix := range fixtures {
		for _, c := range perFixture[fix] {
			p, r, f1 := precisionRecallF1(c.TP, c.FP, c.FN)
			notes := ""
			switch {
			case c.NoSignal:
				notes = "no-signal"
			case c.Skipped:
				notes = "skipped(<50b)"
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %.2f | %.2f | %.2f | %d | %s |\n",
				fix, c.Extractor, c.TP, c.FP, c.FN, p, r, f1, c.TimeNanos/1000, notes)
		}
	}
	fmt.Fprintln(&b, "")

	return b.String()
}
