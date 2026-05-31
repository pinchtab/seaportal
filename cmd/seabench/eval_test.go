package main

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrecisionRecallF1_HandlesZero covers the zero-denominator cases that
// would otherwise NaN: no positives at all (P/R/F1 all 0), TP=0 with FN>0
// (R=0, F1=0), TP=0 with FP>0 (P=0, F1=0), and a sanity case for TP only.
func TestPrecisionRecallF1_HandlesZero(t *testing.T) {
	tests := []struct {
		name         string
		tp, fp, fn   int
		wantP, wantR float64
		wantF1       float64
	}{
		{"all zero", 0, 0, 0, 0, 0, 0},
		{"tp=0, fp=0, fn>0", 0, 0, 5, 0, 0, 0},
		{"tp=0, fp>0, fn=0", 0, 3, 0, 0, 0, 0},
		{"perfect", 4, 0, 0, 1, 1, 1},
		{"half precision, full recall", 2, 2, 0, 0.5, 1, 2.0 / 3.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, r, f1 := precisionRecallF1(tt.tp, tt.fp, tt.fn)
			if !almostEqual(p, tt.wantP) || !almostEqual(r, tt.wantR) || !almostEqual(f1, tt.wantF1) {
				t.Fatalf("got (%.4f, %.4f, %.4f), want (%.4f, %.4f, %.4f)",
					p, r, f1, tt.wantP, tt.wantR, tt.wantF1)
			}
		})
	}
}

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// writeMiniCorpus writes two HTML fixtures + a corpus.yaml under dir and
// returns the corpus path. The fixtures are large enough that strip-tags
// + readability + html-to-markdown all clear the 50-byte skip threshold.
func writeMiniCorpus(t *testing.T, dir string) string {
	t.Helper()
	fixDir := filepath.Join(dir, "testdata")
	if err := os.MkdirAll(fixDir, 0o755); err != nil {
		t.Fatal(err)
	}
	htmlA := `<!doctype html><html><head><title>Alpha</title></head><body>
<article><h1>Alpha</h1><p>This is the alpha article body. Lorem ipsum dolor sit amet, consectetur adipiscing elit.</p></article>
<aside>Subscribe to our newsletter for daily updates and offers from sponsors.</aside>
</body></html>`
	htmlB := `<!doctype html><html><head><title>Beta</title></head><body>
<article><h1>Beta</h1><p>Beta content goes here with enough words to count as a real paragraph for evaluation purposes.</p></article>
<footer>Cookie consent required. Sign up for emails. Subscribe now.</footer>
</body></html>`
	if err := os.WriteFile(filepath.Join(fixDir, "a.html"), []byte(htmlA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixDir, "b.html"), []byte(htmlB), 0o644); err != nil {
		t.Fatal(err)
	}
	corpus := `- path: testdata/a.html
  must_include:
    - "Alpha"
    - "alpha article body"
  must_exclude:
    - "Subscribe"
  expect_class: static
- path: testdata/b.html
  must_include:
    - "Beta"
    - "Beta content goes here"
  must_exclude:
    - "Cookie consent"
    - "Sign up"
  expect_class: static
`
	corpusPath := filepath.Join(dir, "corpus.yaml")
	if err := os.WriteFile(corpusPath, []byte(corpus), 0o644); err != nil {
		t.Fatal(err)
	}
	// Provide a fake go.mod so resolveRepoRoot picks `dir`.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return corpusPath
}

// TestSeabenchEval_Smoke drives runEval against a tiny in-memory corpus and
// asserts the report file exists, is non-trivial, and contains the headline
// table header. Exercises the full extractor stack except subprocess exec.
func TestSeabenchEval_Smoke(t *testing.T) {
	dir := t.TempDir()
	corpusPath := writeMiniCorpus(t, dir)
	reportDir := filepath.Join(dir, "reports")
	runEval([]string{"--corpus", corpusPath, "--report-dir", reportDir})

	entries, err := os.ReadDir(reportDir)
	if err != nil {
		t.Fatal(err)
	}
	var reportPath string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "eval_") && strings.HasSuffix(e.Name(), ".md") {
			reportPath = filepath.Join(reportDir, e.Name())
			break
		}
	}
	if reportPath == "" {
		t.Fatalf("no eval_*.md report in %s; got: %v", reportDir, entries)
	}
	body, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	wantSubstrings := []string{
		"# SeaPortal Eval Bake-off",
		"| Extractor | Precision | Recall | F1 | Time ratio | Skipped (n/total) |",
		"seaportal",
		"strip-tags",
		"readability",
		"html-to-markdown",
		"testdata/a.html",
		"testdata/b.html",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("report missing %q", want)
		}
	}
}

// TestSeabenchEval_IntegrationExec builds the binary and invokes
// `seabench eval --corpus ...` to verify the dispatch layer + exit code +
// on-disk side-effects from an honest end-to-end run.
func TestSeabenchEval_IntegrationExec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration build in -short mode")
	}
	dir := t.TempDir()
	corpusPath := writeMiniCorpus(t, dir)
	reportDir := filepath.Join(dir, "reports")
	binPath := filepath.Join(dir, "seabench")

	build := exec.Command("go", "build", "-o", binPath, "github.com/pinchtab/seaportal/cmd/seabench")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}

	cmd := exec.Command(binPath, "eval", "--corpus", corpusPath, "--report-dir", reportDir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("seabench eval: %v", err)
	}

	entries, err := os.ReadDir(reportDir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "eval_") && strings.HasSuffix(e.Name(), ".md") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no eval_*.md report in %s after exec", reportDir)
	}
}
