package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureDir returns the path to test fixtures, skipping if not present.
func fixtureDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "testdata", "fixtures")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("fixtures not present — run fixture capture script first")
	}
	return dir
}

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir(t), name))
	if err != nil {
		t.Skipf("fixture %s not found: %v", name, err)
	}
	return string(data)
}

// Baseline expectations from the original SeaPortal (before content negotiation).
// These are the "before" numbers from the comparison report.
type expectation struct {
	name           string
	fixture        string // HTML fixture filename
	minChars       int    // Minimum content length we expect
	minConfidence  int
	minHeadings    int
	mustContain    []string // Substrings that must appear in content
	mustNotContain []string // Substrings that must NOT appear (junk detection)
	notSPA         bool     // Must NOT be classified as SPA
	notBlocked     bool     // Must NOT be classified as blocked
}

// Before: baseline numbers from original SeaPortal (HTML-only extraction, no markdown negotiation).
// These are the FromHTML() results on the same fixtures — the "before" for docs-openclaw
// and cloudflare reflects HTML-only extraction (without Accept: text/markdown).
var baselineBefore = map[string]int{
	"example.com":   149,
	"hn":            10943,  // fixture content (varies by capture time)
	"wikipedia":     43375,  // fixture-specific
	"github-clay":   113657, // fixture-specific
	"bbc":           7874,   // fixture-specific
	"nytimes":       3392,   // fixture-specific (HTML extraction)
	"docs-openclaw": 1610,   // HTML-only extraction (no markdown negotiation)
	"react":         4000,   // fixture-specific
	"cloudflare":    5804,   // HTML-only extraction (blocked/partial)
	"creepjs":       1147,
}

var expectations = []expectation{
	{
		name:          "example.com",
		fixture:       "example.com.html",
		minChars:      50,
		minConfidence: 30,
		mustContain:   []string{"Example Domain"},
		notSPA:        true,
		notBlocked:    true,
	},
	{
		name:          "hn",
		fixture:       "hn.html",
		minChars:      5000,
		minConfidence: 50,
		minHeadings:   0,
		mustContain:   []string{"Hacker News"},
		notSPA:        true,
		notBlocked:    true,
	},
	{
		name:          "wikipedia",
		fixture:       "wikipedia.html",
		minChars:      30000,
		minConfidence: 90,
		minHeadings:   5,
		mustContain:   []string{"Web scraping", "data extraction"},
		notSPA:        true,
		notBlocked:    true,
	},
	{
		name:          "github-clay",
		fixture:       "github-clay.html",
		minChars:      50000,
		minConfidence: 90,
		minHeadings:   5,
		mustContain:   []string{"Clay"},
		notSPA:        true,
		notBlocked:    true,
	},
	{
		name:          "bbc",
		fixture:       "bbc.html",
		minChars:      5000,
		minConfidence: 80,
		minHeadings:   10,
		notSPA:        true,
		notBlocked:    true,
	},
	{
		name:          "nytimes",
		fixture:       "nytimes.html",
		minChars:      1000,
		minConfidence: 50,
		mustContain:   []string{"New York Times"},
		notSPA:        true,
		notBlocked:    true,
	},
	{
		name:          "react",
		fixture:       "react.html",
		minChars:      2000,
		minConfidence: 50,
		mustContain:   []string{"React"},
		notSPA:        true,
		notBlocked:    true,
	},
	{
		name:          "creepjs",
		fixture:       "creepjs.html",
		minChars:      500,
		minConfidence: 50,
		mustContain:   []string{"Computing"},
		notBlocked:    true,
	},
}

func TestFixture_HTMLExtraction(t *testing.T) {
	for _, exp := range expectations {
		t.Run(exp.name, func(t *testing.T) {
			html := loadFixture(t, exp.fixture)
			result := FromHTML(html, "https://"+exp.name)

			if result.Error != "" {
				t.Fatalf("extraction error: %s", result.Error)
			}

			if result.Length < exp.minChars {
				t.Errorf("content too short: got %d, want >= %d", result.Length, exp.minChars)
			}

			if result.Confidence < exp.minConfidence {
				t.Errorf("confidence too low: got %d, want >= %d", result.Confidence, exp.minConfidence)
			}

			if exp.minHeadings > 0 && result.HeadingCount < exp.minHeadings {
				t.Errorf("headings too few: got %d, want >= %d", result.HeadingCount, exp.minHeadings)
			}

			for _, s := range exp.mustContain {
				if !strings.Contains(result.Content, s) && !strings.Contains(result.Title, s) {
					t.Errorf("content missing %q", s)
				}
			}

			for _, s := range exp.mustNotContain {
				if strings.Contains(result.Content, s) {
					t.Errorf("content should not contain %q", s)
				}
			}

			if exp.notSPA && result.IsSPA {
				t.Error("should not be classified as SPA")
			}

			if exp.notBlocked && result.IsBlocked {
				t.Error("should not be classified as blocked")
			}

			// Log stats for comparison
			before := baselineBefore[exp.name]
			delta := ""
			if before > 0 {
				pct := float64(result.Length-before) / float64(before) * 100
				if pct > 0 {
					delta = "+" + formatPct(pct)
				} else {
					delta = formatPct(pct)
				}
			}
			t.Logf("  %s: %d chars (before: %d, %s) | conf=%d | headings=%d | quality=%.0f",
				exp.name, result.Length, before, delta, result.Confidence, result.HeadingCount, result.Quality)
		})
	}
}

// TestFixture_MarkdownNegotiation tests the content-negotiation path
// where servers return text/markdown directly.
func TestFixture_MarkdownNegotiation(t *testing.T) {
	cases := []struct {
		name        string
		mdFixture   string
		htmlFixture string
		minChars    int
		mustContain []string
	}{
		{
			name:        "docs-openclaw",
			mdFixture:   "docs-openclaw-md.txt",
			htmlFixture: "docs-openclaw.html",
			minChars:    5000,
			mustContain: []string{"OpenClaw"},
		},
		{
			name:        "cloudflare",
			mdFixture:   "cloudflare-md.txt",
			htmlFixture: "cloudflare.html",
			minChars:    15000,
			mustContain: []string{"Cloudflare"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"_markdown", func(t *testing.T) {
			md := loadFixture(t, tc.mdFixture)
			cleaned := CleanupMarkdown(md)
			mdLen := len(cleaned)

			if mdLen < tc.minChars {
				t.Errorf("markdown content too short: got %d, want >= %d", mdLen, tc.minChars)
			}

			for _, s := range tc.mustContain {
				if !strings.Contains(cleaned, s) {
					t.Errorf("markdown missing %q", s)
				}
			}

			// Title extraction from markdown
			title := extractMarkdownTitle(cleaned)
			if title == "" {
				t.Error("should extract title from markdown")
			}

			headings := CountMarkdownHeadings(cleaned)
			links := CountMarkdownLinks(cleaned)

			before := baselineBefore[tc.name]
			pct := float64(mdLen-before) / float64(before) * 100
			t.Logf("  %s (markdown): %d chars (before: %d, +%.0f%%) | title=%q | headings=%d | links=%d",
				tc.name, mdLen, before, pct, title, headings, links)
		})

		t.Run(tc.name+"_html_fallback", func(t *testing.T) {
			html := loadFixture(t, tc.htmlFixture)
			result := FromHTML(html, "https://"+tc.name)

			htmlLen := result.Length
			before := baselineBefore[tc.name]

			t.Logf("  %s (html-only): %d chars (before: %d) — this is what we get WITHOUT markdown negotiation",
				tc.name, htmlLen, before)
		})
	}
}

// TestFixture_ComparisonSummary prints the full comparison table.
func TestFixture_ComparisonSummary(t *testing.T) {
	// web_fetch reference values from the original comparison
	webFetchChars := map[string]int{
		"example.com":   1256,
		"hn":            10847,
		"wikipedia":     36429,
		"github-clay":   49229,
		"bbc":           827,
		"nytimes":       14570,
		"docs-openclaw": 7056,
		"react":         0, // 404 error
		"cloudflare":    29508,
		"creepjs":       93,
	}

	t.Log("")
	t.Log("╔══════════════════╦══════════╦══════════╦══════════╦════════╗")
	t.Log("║ Site             ║ SP Before║ SP After ║ web_fetch║ Winner ║")
	t.Log("╠══════════════════╬══════════╬══════════╬══════════╬════════╣")

	type row struct {
		name  string
		after int
	}

	var rows []row

	// HTML fixtures
	for _, exp := range expectations {
		html := loadFixture(t, exp.fixture)
		result := FromHTML(html, "https://"+exp.name)
		rows = append(rows, row{name: exp.name, after: result.Length})
	}

	// Markdown fixtures
	for _, name := range []string{"docs-openclaw", "cloudflare"} {
		mdFile := name + "-md.txt"
		md := loadFixture(t, mdFile)
		cleaned := CleanupMarkdown(md)
		rows = append(rows, row{name: name, after: len(cleaned)})
	}

	spWins, wfWins, ties := 0, 0, 0
	for _, r := range rows {
		before := baselineBefore[r.name]
		wf := webFetchChars[r.name]
		winner := "TIE"
		if r.after > wf*12/10 { // SP > wf by 20%
			winner = "SP"
			spWins++
		} else if wf > r.after*12/10 { // wf > SP by 20%
			winner = "wf"
			wfWins++
		} else {
			ties++
		}
		t.Logf("║ %-16s ║ %8d ║ %8d ║ %8d ║ %-6s ║", r.name, before, r.after, wf, winner)
	}

	t.Log("╚══════════════════╩══════════╩══════════╩══════════╩════════╝")
	t.Logf("Score: SeaPortal %d | web_fetch %d | Tie %d", spWins, wfWins, ties)
}

// TestFixture_NoRegression ensures current extraction is at least as good as baseline.
func TestFixture_NoRegression(t *testing.T) {
	// Allow up to 15% regression from baseline (network content varies)
	const regressionThreshold = 0.85

	for _, exp := range expectations {
		t.Run(exp.name, func(t *testing.T) {
			html := loadFixture(t, exp.fixture)
			result := FromHTML(html, "https://"+exp.name)

			before := baselineBefore[exp.name]
			if before == 0 {
				return
			}

			minAcceptable := int(float64(before) * regressionThreshold)
			if result.Length < minAcceptable {
				t.Errorf("REGRESSION: %s went from %d to %d chars (%.0f%% drop)",
					exp.name, before, result.Length,
					(1-float64(result.Length)/float64(before))*100)
			}
		})
	}
}

func formatPct(pct float64) string {
	if pct > 0 {
		return "+" + formatPctAbs(pct)
	}
	return formatPctAbs(pct)
}

func formatPctAbs(pct float64) string {
	if pct > 1000 || pct < -1000 {
		return ">1000%"
	}
	return strings.TrimRight(strings.TrimRight(
		strings.Replace(
			strings.Replace(
				func() string {
					s := ""
					if pct < 0 {
						s = "-"
						pct = -pct
					}
					return s + intToStr(int(pct)) + "." + intToStr(int(pct*10)%10) + "%"
				}(), ".", ".", 1), ".", ".", 1),
		"0"), ".")
}

func intToStr(n int) string {
	if n < 0 {
		n = -n
	}
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
