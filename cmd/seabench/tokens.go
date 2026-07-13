package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/pinchtab/seaportal"
	"github.com/pinchtab/seaportal/internal/corpus"
)

var tokenModes = []seaportal.LinkRetention{
	seaportal.LinkRetentionAll,
	seaportal.LinkRetentionNone,
	seaportal.LinkRetentionText,
	seaportal.LinkRetentionFooter,
}

var tokenModeNames = []string{"all", "none", "text", "footer"}

type ModeStats struct {
	OutputTokens int     `json:"output_tokens"`
	Ratio        float64 `json:"ratio"`
}

type FixtureTokens struct {
	Path         string               `json:"path"`
	SourceTokens int                  `json:"source_tokens"`
	Modes        map[string]ModeStats `json:"modes"`
}

type ModeAggregate struct {
	MeanRatio   float64 `json:"mean_ratio"`
	MedianRatio float64 `json:"median_ratio"`
	P95Ratio    float64 `json:"p95_ratio"`
}

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

	_, mdPath, err := emitReports(*output, "tokens", report, renderTokensMarkdown(report))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("tokens: all-mode mean ratio=%.3f (%d fixtures × %d modes). See %s\n",
		report.PerMode["all"].MeanRatio, report.TotalFixtures, len(tokenModes), mdPath)
}

func tokensCorpus(corpusPath string) (TokensReport, error) {
	var empty TokensReport

	var rows []FixtureTokens
	ratios := make(map[string][]float64, len(tokenModeNames))

	err := forEachFixture(corpusPath, func(entry corpus.Entry, html, baseURL string) error {
		sourceTokens := approxTokenCount(html)

		modes := make(map[string]ModeStats, len(tokenModes))
		for i, mode := range tokenModes {
			name := tokenModeNames[i]
			result := seaportal.FromHTMLWithOptions(html, baseURL, seaportal.Options{LinkRetention: mode})
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
		return nil
	})
	if err != nil {
		return empty, err
	}

	perMode := make(map[string]ModeAggregate, len(tokenModeNames))
	for _, name := range tokenModeNames {
		perMode[name] = ModeAggregate{
			MeanRatio:   mean(ratios[name]),
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

func renderTokensMarkdown(r TokensReport) string {
	var b strings.Builder
	reportHeader(&b, "SeaPortal Token-Efficiency Report",
		"Captured", r.CapturedAt,
		"Git SHA", "`"+r.GitSHA+"`",
		"Corpus", "`"+r.Corpus+"`")
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
