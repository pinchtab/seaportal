package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pinchtab/seaportal"
)

type sweepTarget struct {
	Rank     int
	URL      string
	Domain   string
	Expected string
}

type SiteResult struct {
	Rank               int     `json:"rank,omitempty"`
	URL                string  `json:"url"`
	Domain             string  `json:"domain,omitempty"`
	Expected           string  `json:"expected,omitempty"`
	Class              string  `json:"class"`
	Outcome            string  `json:"outcome"`
	Decision           string  `json:"decision"`
	BrowserRecommended bool    `json:"browserRecommended"`
	Confidence         int     `json:"confidence"`
	Quality            float64 `json:"quality"`
	Length             int     `json:"length"`
	StatusCode         int     `json:"statusCode"`
	TimeMs             int64   `json:"timeMs"`
	IsBlocked          bool    `json:"isBlocked"`
	IsSPA              bool    `json:"isSpa"`
	OK                 bool    `json:"ok"`
	TimedOut           bool    `json:"timedOut,omitempty"`
	Error              string  `json:"error,omitempty"`
}

type LatencyStats struct {
	N    int   `json:"n"`
	P50  int64 `json:"p50"`
	P90  int64 `json:"p90"`
	P95  int64 `json:"p95"`
	P99  int64 `json:"p99"`
	Mean int64 `json:"mean"`
	Max  int64 `json:"max"`
}

type SweepReport struct {
	Version            int                        `json:"version"`
	CapturedAt         string                     `json:"captured_at"`
	GitSHA             string                     `json:"git_sha"`
	SitesFile          string                     `json:"sites_file"`
	Concurrency        int                        `json:"concurrency"`
	TimeoutSec         float64                    `json:"timeout_sec"`
	Fast               bool                       `json:"fast"`
	Total              int                        `json:"total"`
	OK                 int                        `json:"ok"`
	Errors             int                        `json:"errors"`
	TimedOut           int                        `json:"timed_out"`
	Blocked            int                        `json:"blocked"`
	BrowserRecommended int                        `json:"browser_recommended"`
	Labelled           int                        `json:"labelled"`
	Accuracy           float64                    `json:"accuracy"`
	Latency            LatencyStats               `json:"latency_ms"`
	ClassDist          map[string]int             `json:"class_dist"`
	OutcomeDist        map[string]int             `json:"outcome_dist"`
	DecisionDist       map[string]int             `json:"decision_dist"`
	PerClass           map[string]PerClassMetrics `json:"per_class,omitempty"`
	Confusion          []ConfusionCell            `json:"confusion,omitempty"`
	Sites              []SiteResult               `json:"sites"`
}

func runSweep(args []string) {
	fs := flag.NewFlagSet("sweep", flag.ExitOnError)
	sitesPath := fs.String("sites", "competitors/top-1000-sites-tranco.csv", "Path to the site list (CSV rank,domain | TSV sites.tsv | one URL/domain per line)")
	output := fs.String("output", "tests/bench/reports", "Output directory for the reports")
	concurrency := fs.Int("concurrency", 16, "Number of sites fetched in parallel")
	timeout := fs.Duration("timeout", 15*time.Second, "Per-site fetch budget")
	limit := fs.Int("limit", 0, "Cap the number of sites (0 = all)")
	fast := fs.Bool("fast", false, "Use FastMode (bail early when a browser is likely needed)")
	scheme := fs.String("scheme", "https", "Scheme to prefix onto bare domains")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *concurrency < 1 {
		*concurrency = 1
	}

	targets, err := parseSiteList(*sitesPath, *scheme)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sweep:", err)
		os.Exit(1)
	}
	if *limit > 0 && *limit < len(targets) {
		targets = targets[:*limit]
	}
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "sweep: no sites parsed from", *sitesPath)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "  sweeping %d sites (concurrency=%d, timeout=%s, fast=%v)\n",
		len(targets), *concurrency, *timeout, *fast)

	results := executeSweep(targets, *concurrency, *timeout, *fast)
	report := buildSweepReport(*sitesPath, *concurrency, *timeout, *fast, results)

	_, mdPath, err := emitReports(*output, "sweep", report, renderSweepMarkdown(report))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("sweep: %d/%d ok, %d blocked, %d errors, %d timed out; p50=%dms p95=%dms. See %s\n",
		report.OK, report.Total, report.Blocked, report.Errors, report.TimedOut,
		report.Latency.P50, report.Latency.P95, mdPath)
}

func parseSiteList(path, scheme string) ([]sweepTarget, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read site list: %w", err)
	}
	var targets []sweepTarget
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.EqualFold(line, "rank,domain") {
			continue
		}

		var rank int
		var raw, expected string

		switch {
		case strings.Contains(line, "\t"):
			f := strings.Split(line, "\t")
			if len(f) < 2 {
				continue
			}
			raw = strings.TrimSpace(f[1])
			if len(f) >= 3 {
				expected = normalizeLabel(f[2])
			}
		case strings.Contains(line, ","):
			f := strings.Split(line, ",")
			if len(f) >= 2 {
				if n, convErr := strconv.Atoi(strings.TrimSpace(f[0])); convErr == nil {
					rank = n
					raw = strings.TrimSpace(f[1])
				} else {
					raw = strings.TrimSpace(f[0])
				}
			} else {
				raw = strings.TrimSpace(f[0])
			}
		default:
			raw = line
		}

		if raw == "" {
			continue
		}
		url := toURL(raw, scheme)
		targets = append(targets, sweepTarget{
			Rank:     rank,
			URL:      url,
			Domain:   domainOf(raw),
			Expected: expected,
		})
	}
	return targets, nil
}

func normalizeLabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "-" || strings.EqualFold(s, "any") {
		return ""
	}
	return s
}

func toURL(s, scheme string) string {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	return scheme + "://" + s
}

func domainOf(s string) string {
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	return s
}

func executeSweep(targets []sweepTarget, concurrency int, timeout time.Duration, fast bool) []SiteResult {
	results := make([]SiteResult, len(targets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var done int64

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = fetchSite(targets[i], timeout, fast)
				if n := atomic.AddInt64(&done, 1); n%50 == 0 {
					fmt.Fprintf(os.Stderr, "  ... %d/%d\n", n, len(targets))
				}
			}
		}()
	}
	for i := range targets {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

func fetchSite(t sweepTarget, timeout time.Duration, fast bool) SiteResult {
	ch := make(chan seaportal.Result, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				ch <- seaportal.Result{Error: fmt.Sprintf("panic: %v", rec)}
			}
		}()
		ch <- seaportal.FromURLWithOptions(t.URL, seaportal.Options{
			Dedupe:            true,
			FastMode:          fast,
			MaxRetries:        1,
			MaxRetryWait:      3 * time.Second,
			TotalRetryTimeout: timeout,
		})
	}()

	base := SiteResult{Rank: t.Rank, URL: t.URL, Domain: t.Domain, Expected: t.Expected}
	select {
	case r := <-ch:
		base.Class = string(r.Profile.Class)
		base.Outcome = string(r.Profile.Outcome)
		base.Decision = string(r.Profile.Decision)
		base.BrowserRecommended = r.Profile.BrowserRecommended
		base.Confidence = r.Confidence
		base.Quality = r.Quality
		base.Length = r.Length
		base.StatusCode = r.StatusCode
		base.TimeMs = r.TimeMs
		base.IsBlocked = r.IsBlocked
		base.IsSPA = r.IsSPA
		base.Error = r.Error
		base.OK = r.Error == ""
		return base
	case <-time.After(timeout + 5*time.Second):
		base.TimedOut = true
		base.Error = "timeout"
		base.Class = "EMPTY"
		return base
	}
}

func buildSweepReport(sitesFile string, concurrency int, timeout time.Duration, fast bool, results []SiteResult) SweepReport {
	classDist := map[string]int{}
	outcomeDist := map[string]int{}
	decisionDist := map[string]int{}
	matrix := map[string]map[string]int{}

	var okCount, errCount, timedOut, blocked, browserRec, labelled int
	latencies := make([]int64, 0, len(results))

	for _, s := range results {
		class := s.Class
		if class == "" {
			class = "EMPTY"
		}
		classDist[class]++
		if s.Outcome != "" {
			outcomeDist[s.Outcome]++
		}
		if s.Decision != "" {
			decisionDist[s.Decision]++
		}
		if s.BrowserRecommended {
			browserRec++
		}
		if s.IsBlocked || class == "blocked" {
			blocked++
		}
		switch {
		case s.TimedOut:
			timedOut++
		case s.OK:
			okCount++
			latencies = append(latencies, s.TimeMs)
		default:
			errCount++
		}
		if s.Expected != "" {
			labelled++
			if _, ok := matrix[s.Expected]; !ok {
				matrix[s.Expected] = map[string]int{}
			}
			matrix[s.Expected][class]++
		}
	}

	report := SweepReport{
		Version:            1,
		CapturedAt:         time.Now().UTC().Format(time.RFC3339),
		GitSHA:             gitSHA(),
		SitesFile:          sitesFile,
		Concurrency:        concurrency,
		TimeoutSec:         timeout.Seconds(),
		Fast:               fast,
		Total:              len(results),
		OK:                 okCount,
		Errors:             errCount,
		TimedOut:           timedOut,
		Blocked:            blocked,
		BrowserRecommended: browserRec,
		Labelled:           labelled,
		Latency:            latencyStats(latencies),
		ClassDist:          classDist,
		OutcomeDist:        outcomeDist,
		DecisionDist:       decisionDist,
		Sites:              results,
	}

	if labelled > 0 {
		report.PerClass = perClassMetrics(matrix)
		report.Confusion = flattenConfusion(matrix)
		correct := 0
		for exp, row := range matrix {
			correct += row[exp]
		}
		report.Accuracy = float64(correct) / float64(labelled)
	}
	return report
}

func latencyStats(xs []int64) LatencyStats {
	stats := LatencyStats{N: len(xs)}
	if len(xs) == 0 {
		return stats
	}
	sorted := append([]int64(nil), xs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var sum int64
	for _, v := range sorted {
		sum += v
	}
	stats.Mean = sum / int64(len(sorted))
	stats.Max = sorted[len(sorted)-1]
	stats.P50 = percentile(sorted, 0.50)
	stats.P90 = percentile(sorted, 0.90)
	stats.P95 = percentile(sorted, 0.95)
	stats.P99 = percentile(sorted, 0.99)
	return stats
}

func renderSweepMarkdown(r SweepReport) string {
	var b strings.Builder
	reportHeader(&b, "SeaPortal Live Sweep",
		"Captured", r.CapturedAt,
		"Git SHA", "`"+r.GitSHA+"`",
		"Sites file", "`"+r.SitesFile+"`")
	fmt.Fprintf(&b, "- Total: %d (concurrency=%d, timeout=%.0fs, fast=%v)\n", r.Total, r.Concurrency, r.TimeoutSec, r.Fast)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Reliability")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Metric | Count | % |")
	fmt.Fprintln(&b, "|---|---|---|")
	pct := func(n int) float64 {
		if r.Total == 0 {
			return 0
		}
		return 100 * float64(n) / float64(r.Total)
	}
	fmt.Fprintf(&b, "| ok | %d | %.1f%% |\n", r.OK, pct(r.OK))
	fmt.Fprintf(&b, "| blocked | %d | %.1f%% |\n", r.Blocked, pct(r.Blocked))
	fmt.Fprintf(&b, "| errors | %d | %.1f%% |\n", r.Errors, pct(r.Errors))
	fmt.Fprintf(&b, "| timed out | %d | %.1f%% |\n", r.TimedOut, pct(r.TimedOut))
	fmt.Fprintf(&b, "| browser recommended | %d | %.1f%% |\n", r.BrowserRecommended, pct(r.BrowserRecommended))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Latency (successful fetches, ms)")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- n=%d  mean=%d  p50=%d  p90=%d  p95=%d  p99=%d  max=%d\n",
		r.Latency.N, r.Latency.Mean, r.Latency.P50, r.Latency.P90, r.Latency.P95, r.Latency.P99, r.Latency.Max)
	fmt.Fprintln(&b)

	writeDist(&b, "pageClass distribution", r.ClassDist, r.Total)
	writeDist(&b, "Outcome distribution", r.OutcomeDist, r.Total)
	writeDist(&b, "Browser-routing decision distribution", r.DecisionDist, r.Total)

	if r.Labelled > 0 {
		fmt.Fprintf(&b, "## Classification accuracy (labelled subset: %d)\n\n", r.Labelled)
		fmt.Fprintf(&b, "- Accuracy: %.4f\n\n", r.Accuracy)
		fmt.Fprintln(&b, "| Class | Support | Precision | Recall | F1 |")
		fmt.Fprintln(&b, "|---|---|---|---|---|")
		for _, c := range classOrder {
			m, ok := r.PerClass[c]
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "| %s | %d | %.3f | %.3f | %.3f |\n", c, m.Support, m.Precision, m.Recall, m.F1)
		}
		fmt.Fprintln(&b)
	} else {
		fmt.Fprintln(&b, "## Classification accuracy")
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "_Unlabelled list — capability sweep only (no ground-truth `expect_class`)._")
		fmt.Fprintln(&b)
	}

	slow := make([]SiteResult, 0, len(r.Sites))
	for _, s := range r.Sites {
		if s.OK {
			slow = append(slow, s)
		}
	}
	sort.Slice(slow, func(i, j int) bool { return slow[i].TimeMs > slow[j].TimeMs })
	fmt.Fprintln(&b, "## Slowest 15 (ok)")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| URL | Class | ms | Conf |")
	fmt.Fprintln(&b, "|---|---|---|---|")
	for i, s := range slow {
		if i >= 15 {
			break
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d |\n", s.URL, s.Class, s.TimeMs, s.Confidence)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Errors & timeouts (sample of 20)")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| URL | Status | Error |")
	fmt.Fprintln(&b, "|---|---|---|")
	shown := 0
	for _, s := range r.Sites {
		if s.OK {
			continue
		}
		fmt.Fprintf(&b, "| %s | %d | %s |\n", s.URL, s.StatusCode, s.Error)
		if shown++; shown >= 20 {
			break
		}
	}
	if shown == 0 {
		fmt.Fprintln(&b, "| _none_ |  |  |")
	}
	fmt.Fprintln(&b)

	return b.String()
}

func writeDist(b *strings.Builder, title string, dist map[string]int, total int) {
	fmt.Fprintf(b, "## %s\n\n", title)
	type kv struct {
		k string
		n int
	}
	rows := make([]kv, 0, len(dist))
	for k, n := range dist {
		rows = append(rows, kv{k, n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].k < rows[j].k
	})
	fmt.Fprintln(b, "| Value | Count | % |")
	fmt.Fprintln(b, "|---|---|---|")
	for _, row := range rows {
		p := 0.0
		if total > 0 {
			p = 100 * float64(row.n) / float64(total)
		}
		fmt.Fprintf(b, "| %s | %d | %.1f%% |\n", row.k, row.n, p)
	}
	fmt.Fprintln(b)
}
