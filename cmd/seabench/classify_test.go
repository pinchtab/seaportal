package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestClassify_TinyCorpusRoundTrip wires a synthetic 3-entry corpus
// (1 static, 1 ssr-ish, 1 blocked) pointing at tiny inline HTML fixtures
// in a tempdir, runs runClassify via the public entrypoint, and asserts
// the JSON report parses and reports total=3 with at least 2 correct.
// Realism budget: one mis-classification is tolerated so the test does
// not break if the classifier reshapes a borderline label.
func TestClassify_TinyCorpusRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Article-shaped static page: real headings + paragraphs so the
	// extractor surfaces enough signal to land on `static`.
	staticHTML := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Static Demo Article</title></head><body>
<h1>Static Demo Article</h1>
<p>This is the first paragraph of a small static article with enough body text to clear the minimal length threshold used by the classifier downstream.</p>
<h2>A second heading</h2>
<p>Second paragraph carries additional content so the readability extractor does not bail out on a sparse body.</p>
<p>Third paragraph keeps the paragraph count high enough to qualify as a static page profile.</p>
</body></html>`

	// SSR-shaped page: multiple headings, multiple paragraphs, length > 1000.
	var ssrBody strings.Builder
	ssrBody.WriteString(`<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>SSR News</title></head><body>`)
	ssrBody.WriteString(`<h1>Top story headline</h1>`)
	ssrBody.WriteString(`<h2>Subhead one</h2>`)
	ssrBody.WriteString(`<h2>Subhead two</h2>`)
	for i := 0; i < 8; i++ {
		ssrBody.WriteString(`<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.</p>`)
	}
	ssrBody.WriteString(`</body></html>`)
	ssrHTML := ssrBody.String()

	// Blocked page: Cloudflare-style challenge body. The classifier's
	// bot-detection heuristics should flip IsBlocked → PageBlocked.
	blockedHTML := `<!DOCTYPE html><html><head><title>Just a moment...</title></head><body>
<h1>Checking your browser before accessing the site.</h1>
<p>This process is automatic. Your browser will redirect to your requested content shortly.</p>
<p>Please enable Cookies and reload the page. DDoS protection by Cloudflare. Ray ID: 1234567890abcdef.</p>
</body></html>`

	staticPath := filepath.Join(dir, "static.html")
	ssrPath := filepath.Join(dir, "ssr.html")
	blockedPath := filepath.Join(dir, "blocked.html")
	mustWrite(t, staticPath, staticHTML)
	mustWrite(t, ssrPath, ssrHTML)
	mustWrite(t, blockedPath, blockedHTML)

	// Absolute paths in the corpus so resolveRepoRoot's repo-relative
	// rebasing is bypassed (tempdir is not under a go.mod).
	corpusYAML := "- path: " + staticPath + "\n  expect_class: static\n" +
		"- path: " + ssrPath + "\n  expect_class: ssr\n" +
		"- path: " + blockedPath + "\n  expect_class: blocked\n"
	corpusPath := filepath.Join(dir, "corpus.yaml")
	mustWrite(t, corpusPath, corpusYAML)

	outDir := filepath.Join(dir, "out")

	// Drive the full subcommand entrypoint so flag parsing + report
	// writing are exercised end-to-end (this is the public seam).
	runClassify([]string{"--corpus", corpusPath, "--output", outDir})

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	var jsonFile string
	gotMD := false
	gotCSV := false
	for _, e := range entries {
		name := e.Name()
		switch {
		case strings.HasSuffix(name, ".json"):
			jsonFile = filepath.Join(outDir, name)
		case strings.HasSuffix(name, ".md"):
			gotMD = true
		case strings.HasSuffix(name, ".csv"):
			gotCSV = true
		}
	}
	if jsonFile == "" || !gotMD || !gotCSV {
		t.Fatalf("expected json+md+csv in %s, got %v", outDir, entries)
	}

	raw, err := os.ReadFile(jsonFile)
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var report ClassifyReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if report.Total != 3 {
		t.Fatalf("total = %d, want 3", report.Total)
	}
	if report.Correct < 2 {
		t.Fatalf("correct = %d, want >= 2 (per_fixture=%+v)", report.Correct, report.PerFixture)
	}
	if report.Version != 1 {
		t.Errorf("version = %d, want 1", report.Version)
	}
	if len(report.PerFixture) != 3 {
		t.Errorf("per_fixture len = %d, want 3", len(report.PerFixture))
	}
}

// TestClassify_SkipsEmptyExpectClass verifies that entries lacking an
// expect_class label are excluded from accuracy math and counted in the
// Skipped bucket — the classifier owner should not be punished for
// unlabelled corpus drift.
func TestClassify_SkipsEmptyExpectClass(t *testing.T) {
	dir := t.TempDir()
	staticPath := filepath.Join(dir, "s.html")
	mustWrite(t, staticPath, `<!DOCTYPE html><html lang="en"><head><title>T</title></head><body><h1>Hello</h1><p>Body content with enough text to extract something meaningful here.</p><p>Another paragraph for shape.</p></body></html>`)
	unlabelled := filepath.Join(dir, "u.html")
	mustWrite(t, unlabelled, `<!DOCTYPE html><html><body><p>unlabelled</p></body></html>`)

	corpus := "- path: " + staticPath + "\n  expect_class: static\n" +
		"- path: " + unlabelled + "\n  expect_class: \"\"\n"
	corpusPath := filepath.Join(dir, "corpus.yaml")
	mustWrite(t, corpusPath, corpus)

	report, err := classifyCorpus(corpusPath)
	if err != nil {
		t.Fatalf("classifyCorpus: %v", err)
	}
	if report.Total != 1 {
		t.Fatalf("total = %d, want 1", report.Total)
	}
	if report.Skipped != 1 {
		t.Fatalf("skipped = %d, want 1", report.Skipped)
	}
}

// TestClassify_MissingFixtureErrors guarantees that a typo'd fixture
// path aborts the run with a descriptive error rather than silently
// dropping the entry and biasing accuracy upward.
func TestClassify_MissingFixtureErrors(t *testing.T) {
	dir := t.TempDir()
	corpus := "- path: " + filepath.Join(dir, "does-not-exist.html") + "\n  expect_class: static\n"
	corpusPath := filepath.Join(dir, "corpus.yaml")
	mustWrite(t, corpusPath, corpus)

	_, err := classifyCorpus(corpusPath)
	if err == nil {
		t.Fatal("expected error for missing fixture, got nil")
	}
	if !strings.Contains(err.Error(), "read fixture") {
		t.Errorf("error = %q, want it to mention 'read fixture'", err.Error())
	}
}

// TestSlugify_DeterministicAndURLSafe pins the synthetic-URL contract:
// same input → same slug, no slashes, lowercase. Re-runs of the
// `classify` subcommand must be byte-stable.
func TestSlugify_DeterministicAndURLSafe(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"testdata/static/article-ldjson.html", "testdata-static-article-ldjson-html"},
		{"testdata/blocked/cf.html", "testdata-blocked-cf-html"},
		{"Mixed/CASE.html", "mixed-case-html"},
	}
	for _, c := range cases {
		got := slugify(c.in)
		if got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
		if strings.ContainsAny(got, "/ ") {
			t.Errorf("slugify(%q) leaked unsafe chars: %q", c.in, got)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
