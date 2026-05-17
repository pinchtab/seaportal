package main

// tokens is the token-efficiency lane of seabench. For every corpus fixture
// it counts source HTML tokens once, then runs engine.FromHTMLWithOptions
// four times — one per LinkRetention mode (all|none|text|footer) — and
// records output Markdown tokens and the output/source ratio.
//
// Token counts use a deterministic whitespace + punctuation approximation
// (see approxTokenCount). The numbers are not GPT-precise but stable across
// runs, which is what matters for spotting refactor-induced bloat.
//
// Observational: emits a JSON + Markdown report; never tweaks the engine.

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
	"unicode"

	"github.com/pinchtab/seaportal/internal/engine"
)

// tokenModes is the canonical order for the four LinkRetention modes — used
// as map iteration order for rendering and for the deterministic per-fixture
// loop so reports are stable run-to-run.
var tokenModes = []engine.LinkRetention{
	engine.LinkRetentionAll,
	engine.LinkRetentionNone,
	engine.LinkRetentionText,
	engine.LinkRetentionFooter,
}

// tokenModeNames mirrors tokenModes for JSON/Markdown keys.
var tokenModeNames = []string{"all", "none", "text", "footer"}

// ModeStats is the per-fixture per-mode slice.
type ModeStats struct {
	OutputTokens int     `json:"output_tokens"`
	Ratio        float64 `json:"ratio"`
}

// FixtureTokens is the per-fixture row of the JSON report.
type FixtureTokens struct {
	Path         string               `json:"path"`
	SourceTokens int                  `json:"source_tokens"`
	Modes        map[string]ModeStats `json:"modes"`
}

// ModeAggregate is the per-mode roll-up across all fixtures.
type ModeAggregate struct {
	MeanRatio   float64 `json:"mean_ratio"`
	MedianRatio float64 `json:"median_ratio"`
	P95Ratio    float64 `json:"p95_ratio"`
}

// TokensReport mirrors the on-disk JSON schema (version 1).
type TokensReport struct {
	Version       int                      `json:"version"`
	CapturedAt    string                   `json:"captured_at"`
	GitSHA        string                   `json:"git_sha"`
	Corpus        string                   `json:"corpus"`
	TotalFixtures int                      `json:"total_fixtures"`
	PerMode       map[string]ModeAggregate `json:"per_mode"`
	PerFixture    []FixtureTokens          `json:"per_fixture"`
}

func runTokens(args []string) {
	fs := flag.NewFlagSet("tokens", flag.ExitOnError)
	corpusPath := fs.String("corpus", "tests/eval/corpus.yaml", "Path to corpus YAML")
	output := fs.String("output", "tests/bench/reports", "Output directory for the reports")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	report, err := tokensCorpus(*corpusPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tokens:", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*output, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir output:", err)
		os.Exit(1)
	}
	ts := time.Now().UTC().Format("20060102-150405")
	jsonPath := filepath.Join(*output, fmt.Sprintf("tokens_%s.json", ts))
	mdPath := filepath.Join(*output, fmt.Sprintf("tokens_%s.md", ts))

	if err := writeTokensJSON(jsonPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "write json:", err)
		os.Exit(1)
	}
	if err := atomicWrite(mdPath, renderTokensMarkdown(report)); err != nil {
		fmt.Fprintln(os.Stderr, "write markdown:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", jsonPath)
	fmt.Println("wrote", mdPath)
	fmt.Printf("tokens: %d fixtures × %d modes. See %s\n",
		report.TotalFixtures, len(tokenModes), mdPath)
}

// tokensCorpus loads the corpus and runs the 4-mode token-ratio benchmark.
// Sequential execution; runtime on the 31-entry corpus is a few seconds.
func tokensCorpus(corpusPath string) (TokensReport, error) {
	var empty TokensReport
	entries, err := engine.LoadCorpus(corpusPath)
	if err != nil {
		return empty, fmt.Errorf("load corpus: %w", err)
	}

	repoRoot := resolveRepoRoot(corpusPath)

	rows := make([]FixtureTokens, 0, len(entries))
	// ratios[modeName] = slice of per-fixture ratios, for aggregate math.
	ratios := make(map[string][]float64, len(tokenModeNames))
	for _, name := range tokenModeNames {
		ratios[name] = make([]float64, 0, len(entries))
	}

	for _, entry := range entries {
		path := entry.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoRoot, path)
		}
		htmlBytes, readErr := os.ReadFile(path)
		if readErr != nil {
			return empty, fmt.Errorf("read fixture %s: %w", entry.Path, readErr)
		}
		html := string(htmlBytes)
		sourceTokens := approxTokenCount(html)
		baseURL := "https://corpus.local/" + slugify(entry.Path)

		modes := make(map[string]ModeStats, len(tokenModes))
		for i, mode := range tokenModes {
			name := tokenModeNames[i]
			result := engine.FromHTMLWithOptions(html, baseURL, engine.Options{LinkRetention: mode})
			outTokens := approxTokenCount(result.Content)
			ratio := 0.0
			if sourceTokens > 0 && outTokens > 0 {
				ratio = float64(outTokens) / float64(sourceTokens)
			}
			modes[name] = ModeStats{OutputTokens: outTokens, Ratio: ratio}
			ratios[name] = append(ratios[name], ratio)
		}
		rows = append(rows, FixtureTokens{
			Path:         entry.Path,
			SourceTokens: sourceTokens,
			Modes:        modes,
		})
	}

	perMode := make(map[string]ModeAggregate, len(tokenModeNames))
	for _, name := range tokenModeNames {
		perMode[name] = ModeAggregate{
			MeanRatio:   meanFloat(ratios[name]),
			MedianRatio: percentile(ratios[name], 0.50),
			P95Ratio:    percentile(ratios[name], 0.95),
		}
	}

	return TokensReport{
		Version:       1,
		CapturedAt:    time.Now().UTC().Format(time.RFC3339),
		GitSHA:        gitSHA(),
		Corpus:        corpusPath,
		TotalFixtures: len(rows),
		PerMode:       perMode,
		PerFixture:    rows,
	}, nil
}

// approxTokenCount returns a deterministic token-count approximation.
//
// Formula: len(strings.Fields(s)) + punctuation_clusters / 4.
// A "punctuation cluster" is a maximal run of unicode.IsPunct runes — so
// "..." counts as one cluster, "(hi)," counts as two. We add a quarter-token
// per cluster because real BPE tokenizers (e.g. cl100k_base) emit roughly
// one extra token per few punctuation events, not one per character.
//
// Not GPT-precise but stable across runs (no map iteration, no randomness)
// and good enough for relative ratios across modes / refactors.
func approxTokenCount(s string) int {
	if s == "" {
		return 0
	}
	words := len(strings.Fields(s))
	clusters := 0
	inCluster := false
	for _, r := range s {
		if unicode.IsPunct(r) {
			if !inCluster {
				clusters++
				inCluster = true
			}
		} else {
			inCluster = false
		}
	}
	return words + clusters/4
}

// meanFloat returns the arithmetic mean. Defined locally so tokens.go is
// self-contained (eval.go's mean takes a different shape).
func meanFloat(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// percentile returns the q-quantile (0..1) of xs using nearest-rank.
// Stable: sorts a copy so the caller's slice is untouched.
func percentile(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	cp := make([]float64, len(xs))
	copy(cp, xs)
	sort.Float64s(cp)
	idx := int(math.Ceil(q*float64(len(cp)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

func writeTokensJSON(path string, r TokensReport) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, string(raw)+"\n")
}

// renderTokensMarkdown produces the human-friendly report:
//   - per-mode aggregate table (mean / median / p95)
//   - top-5 worst-compression fixtures per mode
//   - side-by-side per-fixture ratios across all four modes
func renderTokensMarkdown(r TokensReport) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# SeaPortal Token-Efficiency Report")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Captured: %s\n", r.CapturedAt)
	fmt.Fprintf(&b, "- Git SHA: `%s`\n", r.GitSHA)
	fmt.Fprintf(&b, "- Corpus: `%s`\n", r.Corpus)
	fmt.Fprintf(&b, "- Fixtures: %d\n", r.TotalFixtures)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Per-mode aggregate ratios")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Mode | Mean | Median | p95 |")
	fmt.Fprintln(&b, "|---|---|---|---|")
	for _, name := range tokenModeNames {
		a := r.PerMode[name]
		fmt.Fprintf(&b, "| %s | %.4f | %.4f | %.4f |\n",
			name, a.MeanRatio, a.MedianRatio, a.P95Ratio)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Worst compression per mode (top 5)")
	fmt.Fprintln(&b)
	for _, name := range tokenModeNames {
		fmt.Fprintf(&b, "### Mode: %s\n\n", name)
		fmt.Fprintln(&b, "| Path | Source tokens | Output tokens | Ratio |")
		fmt.Fprintln(&b, "|---|---|---|---|")
		ordered := make([]FixtureTokens, len(r.PerFixture))
		copy(ordered, r.PerFixture)
		sort.SliceStable(ordered, func(i, j int) bool {
			return ordered[i].Modes[name].Ratio > ordered[j].Modes[name].Ratio
		})
		limit := 5
		if len(ordered) < limit {
			limit = len(ordered)
		}
		for _, row := range ordered[:limit] {
			m := row.Modes[name]
			fmt.Fprintf(&b, "| `%s` | %d | %d | %.4f |\n",
				row.Path, row.SourceTokens, m.OutputTokens, m.Ratio)
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, "## Side-by-side per-fixture ratios")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Path | Source | all | none | text | footer |")
	fmt.Fprintln(&b, "|---|---|---|---|---|---|")
	// Keep corpus order so re-runs diff cleanly.
	for _, row := range r.PerFixture {
		fmt.Fprintf(&b, "| `%s` | %d | %.4f | %.4f | %.4f | %.4f |\n",
			row.Path,
			row.SourceTokens,
			row.Modes["all"].Ratio,
			row.Modes["none"].Ratio,
			row.Modes["text"].Ratio,
			row.Modes["footer"].Ratio,
		)
	}
	return b.String()
}
