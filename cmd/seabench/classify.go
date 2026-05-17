package main

// classify is the page-classification accuracy lane of seabench. It runs
// every corpus fixture through engine.FromHTML, compares the resulting
// result.Profile.Class against the corpus's expect_class label, and emits
// per-class precision/recall/F1 plus a confusion matrix.
//
// Why a separate command:
//   - `eval` scores extraction quality via must_include/must_exclude.
//   - `classify` scores the orthogonal axis: "did the classifier correctly
//     decide whether the page is static / ssr / spa / blocked / etc.".
//
// No network, no LLM, no subprocess: every fixture is on disk; FromHTML is
// pure. The synthetic baseURL is deterministic so re-runs are stable.

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pinchtab/seaportal/internal/engine"
)

// classOrder controls the canonical row/column order for the confusion
// matrix and the per-class P/R/F1 table. Includes the special "EMPTY"
// bucket for fixtures whose profile pipeline did not populate Class.
var classOrder = []string{
	"static",
	"ssr",
	"hydrated",
	"spa",
	"dynamic",
	"blocked",
	"EMPTY",
}

// PerClassMetrics is the per-class slice of the JSON report.
type PerClassMetrics struct {
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
	Support   int     `json:"support"`
}

// ConfusionCell is one cell of the row-expected/column-predicted matrix.
type ConfusionCell struct {
	Expected  string `json:"expected"`
	Predicted string `json:"predicted"`
	Count     int    `json:"count"`
}

// FixtureRow is the per-fixture detail emitted in the JSON report.
type FixtureRow struct {
	Path           string   `json:"path"`
	Expected       string   `json:"expected"`
	Predicted      string   `json:"predicted"`
	Correct        bool     `json:"correct"`
	HeadingCount   int      `json:"heading_count"`
	ParagraphCount int      `json:"paragraph_count"`
	Length         int      `json:"length"`
	Confidence     int      `json:"confidence"`
	IsSPA          bool     `json:"is_spa"`
	IsBlocked      bool     `json:"is_blocked"`
	SPASignals     []string `json:"spa_signals,omitempty"`
}

// ClassifyReport mirrors the on-disk JSON schema (version 1). Field tags
// use snake_case for jq / dashboard friendliness.
type ClassifyReport struct {
	Version    int                        `json:"version"`
	CapturedAt string                     `json:"captured_at"`
	GitSHA     string                     `json:"git_sha"`
	Corpus     string                     `json:"corpus"`
	Total      int                        `json:"total"`
	Correct    int                        `json:"correct"`
	Skipped    int                        `json:"skipped"`
	Accuracy   float64                    `json:"accuracy"`
	PerClass   map[string]PerClassMetrics `json:"per_class"`
	Confusion  []ConfusionCell            `json:"confusion"`
	PerFixture []FixtureRow               `json:"per_fixture"`
}

func runClassify(args []string) {
	fs := flag.NewFlagSet("classify", flag.ExitOnError)
	corpusPath := fs.String("corpus", "tests/eval/corpus.yaml", "Path to corpus YAML")
	output := fs.String("output", "tests/bench/reports", "Output directory for the reports")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	report, err := classifyCorpus(*corpusPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "classify:", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*output, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir output:", err)
		os.Exit(1)
	}
	ts := time.Now().UTC().Format("20060102-150405")
	jsonPath := filepath.Join(*output, fmt.Sprintf("classify_%s.json", ts))
	mdPath := filepath.Join(*output, fmt.Sprintf("classify_%s.md", ts))
	csvPath := filepath.Join(*output, fmt.Sprintf("classify_%s.csv", ts))

	if err := writeClassifyJSON(jsonPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "write json:", err)
		os.Exit(1)
	}
	if err := atomicWrite(mdPath, renderClassifyMarkdown(report)); err != nil {
		fmt.Fprintln(os.Stderr, "write markdown:", err)
		os.Exit(1)
	}
	if err := writeClassifyCSV(csvPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "write csv:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", jsonPath)
	fmt.Println("wrote", mdPath)
	fmt.Println("wrote", csvPath)
	fmt.Printf("classify: %d/%d correct (accuracy=%.3f). See %s\n",
		report.Correct, report.Total, report.Accuracy, mdPath)
}

// classifyCorpus loads the corpus, runs every labelled fixture through
// FromHTML, and produces a fully-populated ClassifyReport. Entries with
// an empty expect_class are skipped with a stderr warning and excluded
// from accuracy math. A failed fixture read aborts the run.
func classifyCorpus(corpusPath string) (ClassifyReport, error) {
	var empty ClassifyReport
	entries, err := engine.LoadCorpus(corpusPath)
	if err != nil {
		return empty, fmt.Errorf("load corpus: %w", err)
	}

	repoRoot := resolveRepoRoot(corpusPath)

	// matrix[expected][predicted] = count. Predicted "EMPTY" is the
	// sentinel for "profile pipeline did not populate Class".
	matrix := make(map[string]map[string]int)
	for _, c := range classOrder {
		matrix[c] = make(map[string]int)
	}

	rows := make([]FixtureRow, 0, len(entries))
	skipped := 0

	for _, entry := range entries {
		if strings.TrimSpace(entry.ExpectClass) == "" {
			fmt.Fprintf(os.Stderr, "skip %s: empty expect_class\n", entry.Path)
			skipped++
			continue
		}
		path := entry.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoRoot, path)
		}
		htmlBytes, readErr := os.ReadFile(path)
		if readErr != nil {
			return empty, fmt.Errorf("read fixture %s: %w", entry.Path, readErr)
		}
		baseURL := "https://corpus.local/" + slugify(entry.Path)
		result := engine.FromHTML(string(htmlBytes), baseURL)

		predicted := string(result.Profile.Class)
		if predicted == "" {
			predicted = "EMPTY"
		}
		expected := entry.ExpectClass

		// Tolerate labels outside classOrder by lazily allocating the
		// row/column; keeps the report honest even if the corpus drifts.
		if _, ok := matrix[expected]; !ok {
			matrix[expected] = make(map[string]int)
		}
		matrix[expected][predicted]++

		rows = append(rows, FixtureRow{
			Path:           entry.Path,
			Expected:       expected,
			Predicted:      predicted,
			Correct:        expected == predicted,
			HeadingCount:   result.HeadingCount,
			ParagraphCount: result.ParagraphCount,
			Length:         result.Length,
			Confidence:     result.Confidence,
			IsSPA:          result.IsSPA,
			IsBlocked:      result.IsBlocked,
			SPASignals:     result.SPASignals,
		})
	}

	total := len(rows)
	correct := 0
	for _, r := range rows {
		if r.Correct {
			correct++
		}
	}

	report := ClassifyReport{
		Version:    1,
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		GitSHA:     gitSHA(),
		Corpus:     corpusPath,
		Total:      total,
		Correct:    correct,
		Skipped:    skipped,
		PerClass:   perClassMetrics(matrix),
		Confusion:  flattenConfusion(matrix),
		PerFixture: rows,
	}
	if total > 0 {
		report.Accuracy = float64(correct) / float64(total)
	}
	return report, nil
}

// perClassMetrics walks the confusion matrix once to produce P/R/F1 and
// support per class. Iterates over the union of row+column keys so any
// drifted-in class still appears.
func perClassMetrics(matrix map[string]map[string]int) map[string]PerClassMetrics {
	keys := classKeys(matrix)
	out := make(map[string]PerClassMetrics, len(keys))
	for _, c := range keys {
		tp := matrix[c][c]
		fp := 0
		for other, row := range matrix {
			if other == c {
				continue
			}
			fp += row[c]
		}
		fn := 0
		for predicted, count := range matrix[c] {
			if predicted == c {
				continue
			}
			fn += count
		}
		support := tp + fn
		p, r, f1 := precisionRecallF1(tp, fp, fn)
		out[c] = PerClassMetrics{
			Precision: p,
			Recall:    r,
			F1:        f1,
			Support:   support,
		}
	}
	return out
}

// classKeys returns every class that appears as either an expected row or
// a predicted column in the matrix, ordered with classOrder first and any
// drifted-in extras sorted alphabetically at the tail.
func classKeys(matrix map[string]map[string]int) []string {
	seen := make(map[string]bool)
	for expected, row := range matrix {
		if len(row) > 0 || hasAnyExpected(matrix, expected) {
			seen[expected] = true
		}
		for predicted := range row {
			seen[predicted] = true
		}
	}
	out := make([]string, 0, len(seen))
	used := make(map[string]bool)
	for _, c := range classOrder {
		if seen[c] {
			out = append(out, c)
			used[c] = true
		}
	}
	extras := make([]string, 0)
	for c := range seen {
		if !used[c] {
			extras = append(extras, c)
		}
	}
	sort.Strings(extras)
	out = append(out, extras...)
	return out
}

// hasAnyExpected reports whether any fixture was labelled with `c` —
// used so empty rows still surface in the per-class table.
func hasAnyExpected(matrix map[string]map[string]int, c string) bool {
	row, ok := matrix[c]
	if !ok {
		return false
	}
	for _, n := range row {
		if n > 0 {
			return true
		}
	}
	return false
}

func flattenConfusion(matrix map[string]map[string]int) []ConfusionCell {
	keys := classKeys(matrix)
	out := make([]ConfusionCell, 0, len(keys)*len(keys))
	for _, exp := range keys {
		for _, pred := range keys {
			n := matrix[exp][pred]
			if n == 0 {
				continue
			}
			out = append(out, ConfusionCell{
				Expected:  exp,
				Predicted: pred,
				Count:     n,
			})
		}
	}
	return out
}

// slugify converts a fixture path into a deterministic URL-safe slug so
// the synthetic baseURL is stable across runs and free of "/" segments
// that would confuse downstream URL parsing.
func slugify(p string) string {
	var b strings.Builder
	b.Grow(len(p))
	for _, r := range strings.ToLower(p) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func writeClassifyJSON(path string, r ClassifyReport) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, string(raw)+"\n")
}

func writeClassifyCSV(path string, r ClassifyReport) error {
	keys := confusionAxes(r)
	tmp := path + ".tmp"
	if err := buildCSV(tmp, keys, r); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// buildCSV is split out so the file lifecycle is local — a deferred
// Close fires on every exit path, satisfying errcheck without scattering
// `_ = f.Close()` placebos through the writer logic.
func buildCSV(tmp string, keys []string, r ClassifyReport) (err error) {
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	w := csv.NewWriter(f)
	header := append([]string{"expected\\predicted"}, keys...)
	if err = w.Write(header); err != nil {
		return err
	}
	// Re-materialise the matrix from the flat cells so the CSV view stays
	// in lockstep with the JSON view (single source of truth = report).
	mat := make(map[string]map[string]int, len(keys))
	for _, c := range keys {
		mat[c] = make(map[string]int)
	}
	for _, cell := range r.Confusion {
		if _, ok := mat[cell.Expected]; !ok {
			mat[cell.Expected] = make(map[string]int)
		}
		mat[cell.Expected][cell.Predicted] = cell.Count
	}
	for _, exp := range keys {
		row := []string{exp}
		for _, pred := range keys {
			row = append(row, fmt.Sprintf("%d", mat[exp][pred]))
		}
		if err = w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// confusionAxes returns the axis (rows = expected, cols = predicted) used
// by both the CSV and the Markdown matrix. Derived from the report's
// confusion cells plus per_class keys so empty rows still render.
func confusionAxes(r ClassifyReport) []string {
	seen := make(map[string]bool)
	for k := range r.PerClass {
		seen[k] = true
	}
	for _, cell := range r.Confusion {
		seen[cell.Expected] = true
		seen[cell.Predicted] = true
	}
	out := make([]string, 0, len(seen))
	used := make(map[string]bool)
	for _, c := range classOrder {
		if seen[c] {
			out = append(out, c)
			used[c] = true
		}
	}
	extras := make([]string, 0)
	for c := range seen {
		if !used[c] {
			extras = append(extras, c)
		}
	}
	sort.Strings(extras)
	return append(out, extras...)
}

func renderClassifyMarkdown(r ClassifyReport) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# SeaPortal Classify Report")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Captured: %s\n", r.CapturedAt)
	fmt.Fprintf(&b, "- Git SHA: `%s`\n", r.GitSHA)
	fmt.Fprintf(&b, "- Corpus: `%s`\n", r.Corpus)
	fmt.Fprintf(&b, "- Total: %d (skipped: %d)\n", r.Total, r.Skipped)
	fmt.Fprintf(&b, "- Correct: %d\n", r.Correct)
	fmt.Fprintf(&b, "- Accuracy: %.4f\n\n", r.Accuracy)

	fmt.Fprintln(&b, "## Per-class metrics")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Class | Support | Precision | Recall | F1 |")
	fmt.Fprintln(&b, "|---|---|---|---|---|")
	axes := confusionAxes(r)
	for _, c := range axes {
		m := r.PerClass[c]
		// A class with zero support AND zero predictions has no meaningful
		// metric; render as N/A so it doesn't drag a "0.000" through the
		// per-class table. Support>0 cases keep numeric formatting.
		hasPredictions := false
		for _, cell := range r.Confusion {
			if cell.Predicted == c {
				hasPredictions = true
				break
			}
		}
		if m.Support == 0 && !hasPredictions {
			fmt.Fprintf(&b, "| %s | 0 | N/A | N/A | N/A |\n", c)
			continue
		}
		fmt.Fprintf(&b, "| %s | %d | %.3f | %.3f | %.3f |\n",
			c, m.Support, m.Precision, m.Recall, m.F1)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Confusion matrix (rows = expected, cols = predicted)")
	fmt.Fprintln(&b)
	// Header
	fmt.Fprint(&b, "| expected \\ predicted |")
	for _, c := range axes {
		fmt.Fprintf(&b, " %s |", c)
	}
	fmt.Fprintln(&b)
	fmt.Fprint(&b, "|---|")
	for range axes {
		fmt.Fprint(&b, "---|")
	}
	fmt.Fprintln(&b)
	// Rebuild matrix from cells for consistent rendering.
	mat := make(map[string]map[string]int, len(axes))
	for _, c := range axes {
		mat[c] = make(map[string]int)
	}
	for _, cell := range r.Confusion {
		if _, ok := mat[cell.Expected]; !ok {
			mat[cell.Expected] = make(map[string]int)
		}
		mat[cell.Expected][cell.Predicted] = cell.Count
	}
	for _, exp := range axes {
		fmt.Fprintf(&b, "| %s |", exp)
		for _, pred := range axes {
			fmt.Fprintf(&b, " %d |", mat[exp][pred])
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Mis-classifications")
	fmt.Fprintln(&b)
	miss := make([]FixtureRow, 0)
	for _, row := range r.PerFixture {
		if !row.Correct {
			miss = append(miss, row)
		}
	}
	if len(miss) == 0 {
		fmt.Fprintln(&b, "_None._")
	} else {
		fmt.Fprintln(&b, "| Path | Expected | Got | Conf | H | P | Len | SPA | Signals |")
		fmt.Fprintln(&b, "|---|---|---|---|---|---|---|---|---|")
		for _, m := range miss {
			fmt.Fprintf(&b, "| `%s` | %s | %s | %d | %d | %d | %d | %v | %s |\n",
				m.Path, m.Expected, m.Predicted, m.Confidence,
				m.HeadingCount, m.ParagraphCount, m.Length, m.IsSPA,
				strings.Join(m.SPASignals, ","))
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Per-fixture diagnostics")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Path | Expected | Got | ✓ | Conf | H | P | Len | SPA | Signals |")
	fmt.Fprintln(&b, "|---|---|---|---|---|---|---|---|---|---|")
	for _, row := range r.PerFixture {
		mark := "✓"
		if !row.Correct {
			mark = "✗"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %d | %d | %d | %d | %v | %s |\n",
			row.Path, row.Expected, row.Predicted, mark, row.Confidence,
			row.HeadingCount, row.ParagraphCount, row.Length, row.IsSPA,
			strings.Join(row.SPASignals, ","))
	}
	return b.String()
}
