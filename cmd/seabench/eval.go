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

// extractor wraps a single extraction implementation under test.
type extractor struct {
	name string
	fn   func(html, baseURL string) (string, error)
}

// scoreCard captures the per-(extractor, fixture) measurement.
//
// TimeNanos is the median wall-clock time across N=3 runs. Skipped fires
// when the extractor produced fewer than 50 chars (treated as failure for
// robustness reporting but excluded from the precision/recall numbers).
// NoSignal fires when the fixture has neither must_include nor must_exclude
// substrings — there's nothing to score, so the entry is reported but
// excluded from aggregate math.
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

// aggregate is the per-extractor headline view, micro-averaged across all
// fixtures with signal.
type aggregate struct {
	Extractor    string
	Precision    float64
	Recall       float64
	F1           float64
	AvgTimeRatio float64
	Skipped      int
	Total        int
}

// runs is the per-fixture repetition count used for median-time estimation.
// Three runs is a cheap compromise — corpus × 4 extractors × 3 runs stays
// in the low-seconds range while damping out single-shot jitter.
const runs = 3

// skipThreshold marks an extractor's output as "skipped" (produced nothing
// useful) when it falls below this many bytes. Chosen to be smaller than
// any realistic article body but larger than incidental boilerplate.
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

	// One-line headline so `./dev bench all` (and a quick eyeball) can read the
	// result without opening the report.
	for _, a := range aggregates {
		if a.Extractor == "seaportal" {
			fmt.Printf("eval: seaportal F1=%.3f (P=%.3f R=%.3f). See %s\n", a.F1, a.Precision, a.Recall, out)
			break
		}
	}
}

// buildExtractors returns the fixed roster of in-process extractors. Order
// matters: strip-tags is last so the headline table renders the baseline at
// the bottom, and time ratios are taken against it explicitly by name.
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
				// Article.Content is HTML; reuse the same html-to-markdown
				// converter the seaportal pipeline uses so the comparison
				// is "readability shape" without seaportal's cleanup pass.
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

// stripTags is the deliberately-dumb baseline. Strips <script>/<style>
// bodies first (otherwise their JS / CSS source dominates the output and
// the must_include hits are pure noise) then nukes every remaining tag and
// collapses whitespace. No entity decoding — that's part of the realism.
func stripTags(html string) string {
	html = scriptRE.ReplaceAllString(html, " ")
	html = styleRE.ReplaceAllString(html, " ")
	html = noscriptRE.ReplaceAllString(html, " ")
	html = commentRE.ReplaceAllString(html, " ")
	html = tagRE.ReplaceAllString(html, " ")
	html = wsRE.ReplaceAllString(html, " ")
	return strings.TrimSpace(html)
}

// scoreCorpus runs every extractor over every fixture, returns the flat
// scoreCard list plus a fixture-indexed map for the per-fixture detail
// table. Charset fixtures reach each extractor as raw bytes (seaportal
// decodes; others may mojibake — that's a real signal, don't fix it).
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
					// Extractor error or panic: treat as empty output for
					// skip detection, but keep timing in for ratio math.
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

// safeExtract calls fn with a recover guard so that an extractor panic on
// one fixture (e.g. seaportal's DetectSPA index-into-multibyte-lowered
// string on certain charset fixtures) does not abort the entire bake-off.
// The panic is reported as an error; the timing collected so far is kept
// in the caller's loop so ratios remain meaningful.
func safeExtract(fn func(html, baseURL string) (string, error), html, baseURL string) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("extractor panic: %v", r)
			out = ""
		}
	}()
	return fn(html, baseURL)
}

// countMatches returns (present, absent) — the count of `needles` that
// appear (TP for must_include / FP for must_exclude) and that don't (FN /
// TN respectively). Empty needle slices yield (0, 0) by definition.
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

// aggregateCards micro-averages TP/FP/FN across all signal-bearing fixtures
// per extractor and computes the mean time ratio relative to strip-tags on
// the same fixtures.
func aggregateCards(cards []scoreCard, extractors []extractor) []aggregate {
	type acc struct {
		tp, fp, fn int
		ratios     []float64
		skipped    int
		total      int
	}
	// Build a fixture → strip-tags-time map first so ratios are deterministic
	// regardless of map iteration order downstream.
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

// precisionRecallF1 computes the three values, returning zero for any of
// them when the denominator would be zero. The "no signal" case (TP=FP=FN=0)
// collapses to 0/0/0, which matches the convention used in the headline.
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
