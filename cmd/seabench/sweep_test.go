package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/seaportal/internal/testserver/fixture"
)

func writeSiteList(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sites.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write site list: %v", err)
	}
	return path
}

func TestParseSiteList_AutoDetectsFormats(t *testing.T) {
	path := writeSiteList(t, strings.Join([]string{
		"# comment line",
		"",
		"rank,domain",
		"1,example.com",
		"docs\thttps://docs.example.com/guide\tstatic\tmarker",
		"news\thttps://news.example.com\tany\t-",
		"bare-domain.org",
		"https://already-url.example.net/page",
	}, "\n"))

	targets, err := parseSiteList(path, "https")
	if err != nil {
		t.Fatalf("parseSiteList: %v", err)
	}
	if len(targets) != 5 {
		t.Fatalf("len(targets) = %d, want 5: %+v", len(targets), targets)
	}

	want := []sweepTarget{
		{Rank: 1, URL: "https://example.com", Domain: "example.com"},
		{URL: "https://docs.example.com/guide", Domain: "docs.example.com", Expected: "static"},
		{URL: "https://news.example.com", Domain: "news.example.com"},
		{URL: "https://bare-domain.org", Domain: "bare-domain.org"},
		{URL: "https://already-url.example.net/page", Domain: "already-url.example.net"},
	}
	for i, w := range want {
		if targets[i] != w {
			t.Errorf("targets[%d] = %+v, want %+v", i, targets[i], w)
		}
	}
}

func TestParseSiteList_SchemePrefixOnBareDomains(t *testing.T) {
	path := writeSiteList(t, "plain.example\n")
	targets, err := parseSiteList(path, "http")
	if err != nil {
		t.Fatalf("parseSiteList: %v", err)
	}
	if len(targets) != 1 || targets[0].URL != "http://plain.example" {
		t.Fatalf("targets = %+v", targets)
	}
}

func TestParseSiteList_MissingFile(t *testing.T) {
	if _, err := parseSiteList(filepath.Join(t.TempDir(), "nope.csv"), "https"); err == nil {
		t.Fatalf("expected error for missing file")
	}
}

const sweepArticleHTML = `<!doctype html><html><head><title>Sweep Fixture</title>
<meta name="description" content="An article used by the sweep lane test.">
</head><body><article><h1>Sweep Fixture</h1>
<p>This article body carries enough readable prose for the extractor to keep
its confidence up. It talks about nothing in particular at a comfortable
length, sentence after sentence, so readability has something to chew on.</p>
<p>A second paragraph rounds out the fixture with more plain text content.</p>
</article></body></html>`

func TestSweepLane_EndToEnd(t *testing.T) {
	srv := fixture.New().
		Route("GET", "/{$}", fixture.Body([]byte(sweepArticleHTML), "text/html; charset=utf-8")).
		Route("GET", "/missing", fixture.Status(404))
	defer srv.Close()

	targets := []sweepTarget{
		{Rank: 1, URL: srv.URL() + "/", Domain: "ok.fixture"},
		{Rank: 2, URL: srv.URL() + "/missing", Domain: "notfound.fixture"},
		{Rank: 3, URL: "ftp://127.0.0.1:1/x", Domain: "err.fixture"},
	}

	results := executeSweep(targets, 2, 10*time.Second, false)
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	ok, notFound, bad := results[0], results[1], results[2]
	if !ok.OK || ok.Error != "" {
		t.Fatalf("healthy site should succeed: %+v", ok)
	}
	if ok.StatusCode != 200 {
		t.Errorf("healthy StatusCode = %d, want 200", ok.StatusCode)
	}
	if ok.Class == "" || ok.Outcome == "" {
		t.Errorf("healthy site should carry class/outcome: %+v", ok)
	}
	if !notFound.OK || notFound.Error != "" {
		t.Fatalf("404 target is expected to count as ok: %+v", notFound)
	}
	if notFound.StatusCode != 404 {
		t.Errorf("404 StatusCode = %d, want 404", notFound.StatusCode)
	}
	if bad.OK || bad.Error == "" {
		t.Fatalf("unfetchable target should fail with an error: %+v", bad)
	}
	if bad.TimedOut {
		t.Errorf("unfetchable target must be an error, not a timeout: %+v", bad)
	}

	report := buildSweepReport("sites.test", 2, 10*time.Second, false, results)
	if report.Version != 1 {
		t.Errorf("Version = %d, want 1", report.Version)
	}
	if report.SitesFile != "sites.test" || report.Concurrency != 2 || report.TimeoutSec != 10 {
		t.Errorf("run parameters not echoed: %+v", report)
	}
	if report.Total != 3 || report.OK != 2 || report.Errors != 1 || report.TimedOut != 0 {
		t.Errorf("counts: total=%d ok=%d errors=%d timedOut=%d, want 3/2/1/0",
			report.Total, report.OK, report.Errors, report.TimedOut)
	}
	if report.Labelled != 0 || report.Accuracy != 0 {
		t.Errorf("unlabelled run must not report accuracy: %+v", report)
	}
	if report.Latency.N != 2 {
		t.Errorf("Latency.N = %d, want 2 (successful fetches only)", report.Latency.N)
	}
	if len(report.ClassDist) == 0 {
		t.Errorf("ClassDist must not be empty")
	}
	if report.CapturedAt == "" {
		t.Errorf("CapturedAt must be set")
	}
	if len(report.Sites) != 3 {
		t.Errorf("Sites rows = %d, want 3", len(report.Sites))
	}

	md := renderSweepMarkdown(report)
	for _, section := range []string{
		"SeaPortal Live Sweep",
		"## Reliability",
		"## Latency (successful fetches, ms)",
		"## pageClass distribution",
		"## Outcome distribution",
		"## Browser-routing decision distribution",
		"_Unlabelled list — capability sweep only",
		"## Slowest 15 (ok)",
		"## Errors & timeouts (sample of 20)",
		"`sites.test`",
	} {
		if !strings.Contains(md, section) {
			t.Errorf("markdown missing %q", section)
		}
	}
	if !strings.Contains(md, "ftp://127.0.0.1:1/x") {
		t.Errorf("markdown error table should list the failing URL")
	}
}

func TestBuildSweepReport_LabelledAccuracy(t *testing.T) {
	results := []SiteResult{
		{URL: "a", Class: "static", Expected: "static", OK: true, TimeMs: 10},
		{URL: "b", Class: "spa", Expected: "spa", OK: true, TimeMs: 20},
		{URL: "c", Class: "static", Expected: "spa", OK: true, TimeMs: 30},
		{URL: "d", Class: "blocked", IsBlocked: true, OK: true, TimeMs: 40},
	}

	report := buildSweepReport("labelled.tsv", 1, time.Second, false, results)
	if report.Labelled != 3 {
		t.Fatalf("Labelled = %d, want 3", report.Labelled)
	}
	if want := 2.0 / 3.0; report.Accuracy != want {
		t.Errorf("Accuracy = %v, want %v", report.Accuracy, want)
	}
	if report.Blocked != 1 {
		t.Errorf("Blocked = %d, want 1", report.Blocked)
	}
	if len(report.PerClass) == 0 || len(report.Confusion) == 0 {
		t.Errorf("labelled run must populate PerClass/Confusion: %+v", report)
	}

	md := renderSweepMarkdown(report)
	if !strings.Contains(md, "## Classification accuracy (labelled subset: 3)") {
		t.Errorf("markdown missing labelled accuracy section")
	}
	if !strings.Contains(md, "| Class | Support | Precision | Recall | F1 |") {
		t.Errorf("markdown missing per-class table header")
	}
}

func TestLatencyStats_Empty(t *testing.T) {
	stats := latencyStats(nil)
	if stats.N != 0 || stats.P50 != 0 || stats.Mean != 0 || stats.Max != 0 {
		t.Errorf("empty input must yield zero stats: %+v", stats)
	}
}

func TestLatencyStats_Percentiles(t *testing.T) {
	stats := latencyStats([]int64{40, 10, 30, 20})
	if stats.N != 4 {
		t.Errorf("N = %d", stats.N)
	}
	if stats.Mean != 25 {
		t.Errorf("Mean = %d, want 25", stats.Mean)
	}
	if stats.Max != 40 {
		t.Errorf("Max = %d, want 40", stats.Max)
	}
	if stats.P50 != 20 {
		t.Errorf("P50 = %d, want 20 (nearest-rank)", stats.P50)
	}
	if stats.P99 != 40 {
		t.Errorf("P99 = %d, want 40", stats.P99)
	}
}
