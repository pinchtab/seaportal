package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	mdtable "github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/go-shiori/go-readability"
	"github.com/pinchtab/seaportal"
	"github.com/pinchtab/seaportal/internal/corpus"
)

type extractor struct {
	name string
	fn   func(html, baseURL string) (string, error)
}

type scoreCard struct {
	Extractor string
	Fixture   string
	TP        int
	FP        int
	FN        int
	TN        int
	TimeNanos int64
	Skipped   bool
	NoSignal  bool
}

type aggregate struct {
	Extractor    string
	Precision    float64
	Recall       float64
	F1           float64
	AvgTimeRatio float64
	Skipped      int
	Total        int
}

const runs = 3

const skipThreshold = 50

func runEval(args []string) {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	corpusPath := fs.String("corpus", "tests/eval/corpus.yaml", "Path to corpus YAML")
	reportDir := fs.String("report-dir", "tests/bench/reports", "Output directory for the report")
	baseline := fs.Bool("baseline", false, "Also write the report to eval_baseline.md (overwrites)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	extractors := buildExtractors()

	cards, perFixture, err := scoreCorpus(*corpusPath, extractors)
	if err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
	aggregates := aggregateCards(cards, extractors)

	report := renderReport(*corpusPath, extractors, aggregates, perFixture)

	ts := time.Now().UTC().Format("20060102-150405")
	if err := os.MkdirAll(*reportDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir report-dir:", err)
		os.Exit(1)
	}
	out := filepath.Join(*reportDir, fmt.Sprintf("eval_%s.md", ts))
	if err := atomicWrite(out, report); err != nil {
		fmt.Fprintln(os.Stderr, "write report:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", out)
	if *baseline {
		baselinePath := filepath.Join(*reportDir, "eval_baseline.md")
		if err := atomicWrite(baselinePath, report); err != nil {
			fmt.Fprintln(os.Stderr, "write baseline:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", baselinePath)
	}

	for _, a := range aggregates {
		if a.Extractor == "seaportal" {
			fmt.Printf("eval: seaportal F1=%.3f (P=%.3f R=%.3f). See %s\n", a.F1, a.Precision, a.Recall, out)
			break
		}
	}
}

func buildExtractors() []extractor {
	htmConv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			mdtable.NewTablePlugin(),
		),
	)
	return []extractor{
		{
			name: "seaportal",
			fn: func(html, baseURL string) (string, error) {
				return seaportal.FromHTML(html, baseURL).Content, nil
			},
		},
		{
			name: "readability",
			fn: func(html, baseURL string) (string, error) {
				parsed, _ := url.Parse(baseURL)
				article, err := readability.FromReader(strings.NewReader(html), parsed)
				if err != nil {
					return "", err
				}
				md, mdErr := htmConv.ConvertString(article.Content)
				if mdErr != nil {
					return article.TextContent, nil
				}
				return md, nil
			},
		},
		{
			name: "html-to-markdown",
			fn: func(html, _ string) (string, error) {
				return htmConv.ConvertString(html)
			},
		},
		{
			name: "strip-tags",
			fn: func(html, _ string) (string, error) {
				return stripTags(html), nil
			},
		},
	}
}

var (
	tagRE      = regexp.MustCompile(`<[^>]+>`)
	wsRE       = regexp.MustCompile(`[\s ]+`)
	scriptRE   = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	styleRE    = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`)
	noscriptRE = regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript\s*>`)
	commentRE  = regexp.MustCompile(`(?s)<!--.*?-->`)
)

func stripTags(html string) string {
	html = scriptRE.ReplaceAllString(html, " ")
	html = styleRE.ReplaceAllString(html, " ")
	html = noscriptRE.ReplaceAllString(html, " ")
	html = commentRE.ReplaceAllString(html, " ")
	html = tagRE.ReplaceAllString(html, " ")
	html = wsRE.ReplaceAllString(html, " ")
	return strings.TrimSpace(html)
}

func scoreCorpus(corpusPath string, extractors []extractor) ([]scoreCard, map[string][]scoreCard, error) {
	var cards []scoreCard
	perFixture := make(map[string][]scoreCard)

	err := forEachFixture(corpusPath, func(entry corpus.Entry, html, baseURL string) error {
		noSignal := len(entry.MustInclude) == 0 && len(entry.MustExclude) == 0

		for _, ex := range extractors {
			card := scoreCard{
				Extractor: ex.name,
				Fixture:   entry.Path,
				NoSignal:  noSignal,
			}
			times := make([]int64, 0, runs)
			var out string
			for i := 0; i < runs; i++ {
				start := time.Now()
				o, exErr := safeExtract(ex.fn, html, baseURL)
				elapsed := time.Since(start).Nanoseconds()
				times = append(times, elapsed)
				if exErr != nil {
					o = ""
				}
				out = o
			}
			card.TimeNanos = percentile(times, 0.50)
			if len(out) < skipThreshold {
				card.Skipped = true
			}
			if !noSignal {
				card.TP, card.FN = countMatches(out, entry.MustInclude)
				card.FP, card.TN = countMatches(out, entry.MustExclude)
			}
			cards = append(cards, card)
			perFixture[entry.Path] = append(perFixture[entry.Path], card)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return cards, perFixture, nil
}

func safeExtract(fn func(html, baseURL string) (string, error), html, baseURL string) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("extractor panic: %v", r)
			out = ""
		}
	}()
	return fn(html, baseURL)
}

func countMatches(haystack string, needles []string) (present, absent int) {
	for _, n := range needles {
		if n == "" {
			continue
		}
		if strings.Contains(haystack, n) {
			present++
		} else {
			absent++
		}
	}
	return present, absent
}

func aggregateCards(cards []scoreCard, extractors []extractor) []aggregate {
	type acc struct {
		tp, fp, fn int
		ratios     []float64
		skipped    int
		total      int
	}
	stripTime := make(map[string]int64)
	for _, c := range cards {
		if c.Extractor == "strip-tags" {
			stripTime[c.Fixture] = c.TimeNanos
		}
	}

	bins := make(map[string]*acc, len(extractors))
	for _, ex := range extractors {
		bins[ex.name] = &acc{}
	}

	for _, c := range cards {
		a := bins[c.Extractor]
		a.total++
		if c.Skipped {
			a.skipped++
		}
		if !c.NoSignal {
			a.tp += c.TP
			a.fp += c.FP
			a.fn += c.FN
		}
		if t, ok := stripTime[c.Fixture]; ok && t > 0 {
			a.ratios = append(a.ratios, float64(c.TimeNanos)/float64(t))
		}
	}

	out := make([]aggregate, 0, len(extractors))
	for _, ex := range extractors {
		a := bins[ex.name]
		p, r, f1 := precisionRecallF1(a.tp, a.fp, a.fn)
		out = append(out, aggregate{
			Extractor:    ex.name,
			Precision:    p,
			Recall:       r,
			F1:           f1,
			AvgTimeRatio: mean(a.ratios),
			Skipped:      a.skipped,
			Total:        a.total,
		})
	}
	return out
}

func precisionRecallF1(tp, fp, fn int) (precision, recall, f1 float64) {
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return precision, recall, f1
}
