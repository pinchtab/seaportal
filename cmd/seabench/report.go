package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

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

	fmt.Fprintf(&b, "# SeaPortal Eval Bake-off\n\n")
	fmt.Fprintf(&b, "- Generated: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Corpus: `%s`\n", corpusPath)
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
