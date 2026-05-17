package main

// diff is the cleanup-pipeline differential lane of seabench. It runs every
// corpus fixture under three pipeline-aggressiveness modes built from existing
// engine.Options toggles, then diffs the per-fixture output of each non-default
// mode against the `default` mode.
//
// Why a separate command:
//   - `eval` scores extraction precision/recall against substring markers.
//   - `classify` scores the page-class decision.
//   - `diff` is purely observational: when you tighten dedupe (or any other
//     cleanup knob) you want a quick visual on which fixtures shifted and by
//     how many chars/lines — without having to commit-and-compare manually.
//
// Three modes (no new engine config; only existing toggles):
//   - minimal:    Dedupe=false, NoNearDedupe=true,  NoPruneFallback=true
//   - default:    zero-value Options (current shipped defaults)
//   - aggressive: Dedupe=true,  NoNearDedupe=false, NoPruneFallback=false
//
// Two comparisons per run: default-vs-minimal and default-vs-aggressive.
// `char_delta = len(default) - len(variant)`; negative means the variant
// produced LESS content (more aggressive filtering).

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pinchtab/seaportal/internal/engine"
)

// diffModeOrder fixes the canonical mode iteration order independent of map
// iteration.
var diffModeOrder = []string{"minimal", "default", "aggressive"}

// diffModeOptions returns the engine.Options for a named diff mode. Centralised
// so the test and the runner share a single source of truth.
func diffModeOptions(mode string) engine.Options {
	switch mode {
	case "minimal":
		return engine.Options{Dedupe: false, NoNearDedupe: true, NoPruneFallback: true}
	case "default":
		return engine.Options{}
	case "aggressive":
		return engine.Options{Dedupe: true, NoNearDedupe: false, NoPruneFallback: false}
	default:
		return engine.Options{}
	}
}

// DiffPerFixture is one fixture's delta inside a comparison.
type DiffPerFixture struct {
	Path         string `json:"path"`
	CharDelta    int    `json:"char_delta"`
	LineDelta    int    `json:"line_delta"`
	FirstDiffIdx int    `json:"first_diff_idx"`
	Snippet      string `json:"snippet,omitempty"`
	BaselineLen  int    `json:"baseline_len"`
	VariantLen   int    `json:"variant_len"`
	Panicked     string `json:"panicked,omitempty"`
}

// DiffComparison groups all per-fixture deltas for one (baseline, variant)
// pair. Aggregates are pre-computed so the Markdown renderer is a pure mapper.
type DiffComparison struct {
	Baseline        string           `json:"baseline"`
	Variant         string           `json:"variant"`
	FixturesChanged int              `json:"fixtures_changed"`
	MeanAbsChar     float64          `json:"mean_abs_char_delta"`
	MaxAbsChar      int              `json:"max_abs_char_delta"`
	PerFixture      []DiffPerFixture `json:"per_fixture"`
}

// DiffReport is the on-disk JSON shape (version 1). snake_case for
// jq / dashboard friendliness.
type DiffReport struct {
	Version      int              `json:"version"`
	CapturedAt   string           `json:"captured_at"`
	GitSHA       string           `json:"git_sha"`
	Corpus       string           `json:"corpus"`
	Modes        []string         `json:"modes"`
	SnippetChars int              `json:"snippet_chars"`
	Comparisons  []DiffComparison `json:"comparisons"`
}

func runDiff(args []string) {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	corpusPath := fs.String("corpus", "tests/eval/corpus.yaml", "Path to corpus YAML")
	output := fs.String("output", "tests/bench/reports", "Output directory for JSON + Markdown reports")
	snippetChars := fs.Int("snippet-chars", 400, "Max chars in the first-divergence snippet for top-3 fixtures")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if *snippetChars < 0 {
		fmt.Fprintln(os.Stderr, "--snippet-chars must be >= 0")
		os.Exit(2)
	}

	report, err := diffCorpus(*corpusPath, *snippetChars)
	if err != nil {
		fmt.Fprintln(os.Stderr, "diff:", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*output, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir output:", err)
		os.Exit(1)
	}
	ts := time.Now().UTC().Format("20060102-150405")
	jsonPath := filepath.Join(*output, fmt.Sprintf("diff_%s.json", ts))
	mdPath := filepath.Join(*output, fmt.Sprintf("diff_%s.md", ts))

	if err := writeDiffJSON(jsonPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "write json:", err)
		os.Exit(1)
	}
	if err := atomicWrite(mdPath, renderDiffMarkdown(report)); err != nil {
		fmt.Fprintln(os.Stderr, "write markdown:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", jsonPath)
	fmt.Println("wrote", mdPath)
}

// diffCorpus loads the corpus, runs every fixture through the three modes,
// and builds the populated DiffReport. Pure: tests call this directly.
func diffCorpus(corpusPath string, snippetChars int) (DiffReport, error) {
	var empty DiffReport
	entries, err := engine.LoadCorpus(corpusPath)
	if err != nil {
		return empty, fmt.Errorf("load corpus: %w", err)
	}
	repoRoot := resolveRepoRoot(corpusPath)

	// outputs[path][mode] = result.Content (or "" if read failed / panicked).
	outputs := make(map[string]map[string]string, len(entries))
	panics := make(map[string]map[string]string, len(entries))
	order := make([]string, 0, len(entries))

	for _, entry := range entries {
		path := entry.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoRoot, path)
		}
		htmlBytes, readErr := os.ReadFile(path)
		if readErr != nil {
			return empty, fmt.Errorf("read fixture %s: %w", entry.Path, readErr)
		}
		baseURL := "https://corpus.local/" + slugify(entry.Path)

		outputs[entry.Path] = make(map[string]string, len(diffModeOrder))
		panics[entry.Path] = make(map[string]string, len(diffModeOrder))
		order = append(order, entry.Path)

		for _, mode := range diffModeOrder {
			content, panicMsg := runOneExtract(string(htmlBytes), baseURL, diffModeOptions(mode))
			outputs[entry.Path][mode] = content
			if panicMsg != "" {
				panics[entry.Path][mode] = panicMsg
			}
		}
	}

	comparisons := []DiffComparison{
		buildComparison("default", "minimal", order, outputs, panics, snippetChars),
		buildComparison("default", "aggressive", order, outputs, panics, snippetChars),
	}

	return DiffReport{
		Version:      1,
		CapturedAt:   time.Now().UTC().Format(time.RFC3339),
		GitSHA:       gitSHA(),
		Corpus:       corpusPath,
		Modes:        append([]string(nil), diffModeOrder...),
		SnippetChars: snippetChars,
		Comparisons:  comparisons,
	}, nil
}

// runOneExtract calls FromHTMLWithOptions inside a recover so a panic in one
// mode (e.g. a regression in cleanup) does not blow up the whole bench. On
// panic the content is treated as empty and the recovered value is recorded.
func runOneExtract(html, baseURL string, opts engine.Options) (content string, panicMsg string) {
	defer func() {
		if r := recover(); r != nil {
			content = ""
			panicMsg = fmt.Sprintf("%v", r)
		}
	}()
	res := engine.FromHTMLWithOptions(html, baseURL, opts)
	return res.Content, ""
}

// buildComparison computes per-fixture deltas for one (baseline, variant)
// pair and rolls up aggregate stats. Snippets are populated only on the
// top-3 fixtures by abs char-delta (sorting is done in renderDiffMarkdown
// for the per-comparison table; here we attach the snippet straight away so
// the JSON also surfaces it for the top-3).
func buildComparison(baseline, variant string, order []string,
	outputs, panics map[string]map[string]string, snippetChars int) DiffComparison {
	rows := make([]DiffPerFixture, 0, len(order))
	for _, p := range order {
		base := outputs[p][baseline]
		var pmsg string
		if panics[p][baseline] != "" {
			pmsg = "baseline:" + panics[p][baseline]
		}
		if panics[p][variant] != "" {
			if pmsg != "" {
				pmsg += "; "
			}
			pmsg += "variant:" + panics[p][variant]
		}
		varc := outputs[p][variant]
		rows = append(rows, DiffPerFixture{
			Path:         p,
			CharDelta:    len(base) - len(varc),
			LineDelta:    countLines(base) - countLines(varc),
			FirstDiffIdx: firstDiffIndex(base, varc),
			BaselineLen:  len(base),
			VariantLen:   len(varc),
			Panicked:     pmsg,
		})
	}

	// Top-3 by abs char-delta get a snippet attached to the JSON row.
	idxByAbs := make([]int, len(rows))
	for i := range rows {
		idxByAbs[i] = i
	}
	sort.SliceStable(idxByAbs, func(i, j int) bool {
		return absInt(rows[idxByAbs[i]].CharDelta) > absInt(rows[idxByAbs[j]].CharDelta)
	})
	topN := 3
	if len(idxByAbs) < topN {
		topN = len(idxByAbs)
	}
	for k := 0; k < topN; k++ {
		idx := idxByAbs[k]
		if rows[idx].FirstDiffIdx < 0 {
			continue
		}
		base := outputs[rows[idx].Path][baseline]
		varc := outputs[rows[idx].Path][variant]
		rows[idx].Snippet = buildDiffSnippet(base, varc, rows[idx].FirstDiffIdx, snippetChars)
	}

	changed := 0
	var sumAbs, maxAbs int
	for _, r := range rows {
		if r.CharDelta != 0 || r.LineDelta != 0 || r.FirstDiffIdx >= 0 {
			changed++
		}
		a := absInt(r.CharDelta)
		sumAbs += a
		if a > maxAbs {
			maxAbs = a
		}
	}
	var mean float64
	if len(rows) > 0 {
		mean = float64(sumAbs) / float64(len(rows))
	}

	return DiffComparison{
		Baseline:        baseline,
		Variant:         variant,
		FixturesChanged: changed,
		MeanAbsChar:     mean,
		MaxAbsChar:      maxAbs,
		PerFixture:      rows,
	}
}

// firstDiffIndex returns the first byte index at which a and b differ,
// or -1 if they are identical.
func firstDiffIndex(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

// buildDiffSnippet returns a short two-sided window around the first
// divergence: a window of width snippetChars/2 on each side, drawn from
// both strings, separated by a marker. Indices are clamped so we never
// slice past the end. Newlines are escaped so the snippet survives
// Markdown table rendering.
func buildDiffSnippet(base, variant string, idx, snippetChars int) string {
	if snippetChars <= 0 {
		return ""
	}
	half := snippetChars / 2
	bs := clampSlice(base, idx-half, idx+half)
	vs := clampSlice(variant, idx-half, idx+half)
	return "baseline@" + fmt.Sprintf("%d", idx) + ": " + escapeForCell(bs) +
		" || variant: " + escapeForCell(vs)
}

func clampSlice(s string, lo, hi int) string {
	if lo < 0 {
		lo = 0
	}
	if hi > len(s) {
		hi = len(s)
	}
	if lo >= hi {
		return ""
	}
	return s[lo:hi]
}

// escapeForCell makes the snippet safe for a single Markdown table cell:
// strip newlines + collapse the pipe character that would close the cell.
func escapeForCell(s string) string {
	r := strings.NewReplacer("\n", "\\n", "\r", "\\r", "|", "\\|", "`", "'")
	return r.Replace(s)
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func writeDiffJSON(path string, r DiffReport) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, string(raw)+"\n")
}

func renderDiffMarkdown(r DiffReport) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# SeaPortal Cleanup-Diff Report")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Captured: %s\n", r.CapturedAt)
	fmt.Fprintf(&b, "- Git SHA: `%s`\n", r.GitSHA)
	fmt.Fprintf(&b, "- Corpus: `%s`\n", r.Corpus)
	fmt.Fprintf(&b, "- Modes: %s\n", strings.Join(r.Modes, ", "))
	fmt.Fprintf(&b, "- Snippet window: %d chars\n\n", r.SnippetChars)
	fmt.Fprintln(&b, "_`char_delta = len(baseline) - len(variant)`. Negative ⇒ variant produced LESS content (more aggressive filtering)._")
	fmt.Fprintln(&b)

	for _, c := range r.Comparisons {
		fmt.Fprintf(&b, "## %s vs %s\n\n", c.Baseline, c.Variant)
		fmt.Fprintf(&b, "- Fixtures with any delta: %d / %d\n", c.FixturesChanged, len(c.PerFixture))
		mean := c.MeanAbsChar
		if math.IsNaN(mean) {
			mean = 0
		}
		fmt.Fprintf(&b, "- Mean abs char-delta: %.1f\n", mean)
		fmt.Fprintf(&b, "- Max abs char-delta: %d\n\n", c.MaxAbsChar)

		// Top-3 by abs char-delta.
		sorted := append([]DiffPerFixture(nil), c.PerFixture...)
		sort.SliceStable(sorted, func(i, j int) bool {
			return absInt(sorted[i].CharDelta) > absInt(sorted[j].CharDelta)
		})
		top := sorted
		if len(top) > 3 {
			top = top[:3]
		}
		fmt.Fprintln(&b, "### Top-3 divergent fixtures")
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "| Path | char_delta | line_delta | first_diff | snippet |")
		fmt.Fprintln(&b, "|---|---|---|---|---|")
		for _, row := range top {
			fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %s |\n",
				row.Path, row.CharDelta, row.LineDelta, row.FirstDiffIdx, row.Snippet)
		}
		fmt.Fprintln(&b)

		// Full table — counts only, no snippets, sorted by abs char-delta desc.
		fmt.Fprintln(&b, "### All fixtures")
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "| Path | char_delta | line_delta | first_diff | baseline_len | variant_len | panicked |")
		fmt.Fprintln(&b, "|---|---|---|---|---|---|---|")
		for _, row := range sorted {
			fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %d | %s |\n",
				row.Path, row.CharDelta, row.LineDelta, row.FirstDiffIdx,
				row.BaselineLen, row.VariantLen, escapeForCell(row.Panicked))
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}
