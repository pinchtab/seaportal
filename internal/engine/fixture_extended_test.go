//go:build smoke

package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

type siteTest struct {
	name         string
	fixture      string
	mdFixture    string
	url          string
	category     string
	minChars     int
	minConf      int
	mustContain  []string
	mustNotSPA   bool
	mustNotBlock bool
}

var extendedSites = []siteTest{
	{name: "example.com", fixture: "example.com.html", url: "https://example.com", category: "static",
		minChars: 50, minConf: 30, mustContain: []string{"Example Domain"}, mustNotSPA: true, mustNotBlock: true},
	{name: "hn", fixture: "hn.html", url: "https://news.ycombinator.com", category: "social",
		minChars: 5000, minConf: 50, mustNotSPA: true, mustNotBlock: true},
	{name: "wikipedia", fixture: "wikipedia.html", url: "https://en.wikipedia.org/wiki/Web_scraping", category: "reference",
		minChars: 30000, minConf: 90, mustContain: []string{"Web scraping"}, mustNotSPA: true, mustNotBlock: true},
	{name: "github-clay", fixture: "github-clay.html", url: "https://github.com/nicbarker/clay", category: "reference",
		minChars: 50000, minConf: 90, mustContain: []string{"Clay"}, mustNotSPA: true, mustNotBlock: true},
	{name: "bbc", fixture: "bbc.html", url: "https://www.bbc.co.uk/news", category: "news",
		minChars: 5000, minConf: 80, mustNotSPA: true, mustNotBlock: true},
	{name: "nytimes", fixture: "nytimes.html", url: "https://www.nytimes.com", category: "news",
		minChars: 1000, minConf: 50, mustContain: []string{"New York Times"}, mustNotSPA: true, mustNotBlock: true},
	{name: "react", fixture: "react.html", url: "https://react.dev", category: "docs",
		minChars: 2000, minConf: 50, mustContain: []string{"React"}, mustNotSPA: true, mustNotBlock: true},
	{name: "creepjs", fixture: "creepjs.html", url: "https://abrahamjuliot.github.io/creepjs/", category: "js-heavy",
		minChars: 500, minConf: 50, mustNotBlock: true},
	{name: "docs-openclaw", mdFixture: "docs-openclaw-md.txt", fixture: "docs-openclaw.html", url: "https://docs.openclaw.ai", category: "docs",
		minChars: 5000, minConf: 80, mustContain: []string{"OpenClaw"}, mustNotBlock: true},
	{name: "cloudflare", mdFixture: "cloudflare-md.txt", fixture: "cloudflare.html", url: "https://www.cloudflare.com", category: "docs",
		minChars: 15000, minConf: 80, mustContain: []string{"Cloudflare"}, mustNotBlock: true},

	{name: "stackoverflow", fixture: "stackoverflow.html", url: "https://stackoverflow.com/questions/11227809", category: "reference",
		minChars: 1000, minConf: 50, mustContain: []string{"sorted array"}, mustNotSPA: true, mustNotBlock: true},
	{name: "medium", fixture: "medium.html", url: "https://medium.com/@karpathy/software-2-0-a64152b37c35", category: "news",
		minChars: 0, minConf: 0},
	{name: "reddit", fixture: "reddit.html", url: "https://old.reddit.com/r/programming/top/?t=week", category: "social",
		minChars: 500, minConf: 50, mustNotSPA: true, mustNotBlock: true},
	{name: "python-docs", fixture: "python-docs.html", url: "https://docs.python.org/3/tutorial/classes.html", category: "docs",
		minChars: 10000, minConf: 80, mustContain: []string{"class"}, mustNotSPA: true, mustNotBlock: true},
	{name: "mdn", fixture: "mdn.html", url: "https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Promise", category: "docs",
		minChars: 5000, minConf: 70, mustContain: []string{"Promise"}, mustNotSPA: true, mustNotBlock: true},
	{name: "stripe-docs", fixture: "stripe-docs.html", mdFixture: "stripe-docs-md.txt", url: "https://docs.stripe.com/payments/accept-a-payment", category: "api-docs",
		minChars: 3000, minConf: 50, mustContain: []string{"payment"}, mustNotBlock: true},
	{name: "arxiv", fixture: "arxiv.html", url: "https://arxiv.org/abs/2401.02954", category: "academic",
		minChars: 500, minConf: 50, mustNotSPA: true, mustNotBlock: true},
	{name: "hn-item", fixture: "hackernews-item.html", url: "https://news.ycombinator.com/item?id=42424242", category: "social",
		minChars: 200, minConf: 40, mustNotSPA: true, mustNotBlock: true},
}

type extractResult struct {
	name           string
	category       string
	chars          int
	confidence     int
	headings       int
	links          int
	paragraphs     int
	quality        float64
	isSPA          bool
	isBlocked      bool
	extractMs      int64
	title          string
	hasStructure   bool
	hasLinks       bool
	contentPreview string
}

func extractFixture(t *testing.T, st siteTest) extractResult {
	t.Helper()

	var content string
	var chars int
	var confidence int
	var headings, links, paragraphs int
	var quality float64
	var isSPA, isBlocked bool
	var title string
	var extractMs int64

	if st.mdFixture != "" {
		md := loadFixture(t, st.mdFixture)
		start := time.Now()
		cleaned := CleanupMarkdown(md)
		extractMs = time.Since(start).Milliseconds()

		content = cleaned
		chars = len(cleaned)
		confidence = 100
		headings = CountMarkdownHeadings(cleaned)
		links = CountMarkdownLinks(cleaned)
		paragraphs = countMarkdownParagraphs(cleaned)
		title = extractMarkdownTitle(cleaned)
		quality = float64(ComputeQuality(cleaned).Score)
	} else {
		html := loadFixture(t, st.fixture)
		start := time.Now()
		result := FromHTML(html, st.url)
		extractMs = time.Since(start).Milliseconds()

		content = result.Content
		chars = result.Length
		confidence = result.Confidence
		headings = result.HeadingCount
		links = result.LinkCount
		paragraphs = result.ParagraphCount
		quality = result.Quality
		isSPA = result.IsSPA
		isBlocked = result.IsBlocked
		title = result.Title
	}

	preview := content
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	preview = strings.ReplaceAll(preview, "\n", " ")

	return extractResult{
		name:           st.name,
		category:       st.category,
		chars:          chars,
		confidence:     confidence,
		headings:       headings,
		links:          links,
		paragraphs:     paragraphs,
		quality:        quality,
		isSPA:          isSPA,
		isBlocked:      isBlocked,
		extractMs:      extractMs,
		title:          title,
		hasStructure:   headings >= 2 && paragraphs >= 3,
		hasLinks:       links >= 3,
		contentPreview: preview,
	}
}

func TestExtended_AllSites(t *testing.T) {
	var results []extractResult

	for _, st := range extendedSites {
		t.Run(st.name, func(t *testing.T) {
			r := extractFixture(t, st)
			results = append(results, r)

			if r.chars < st.minChars {
				t.Errorf("content too short: %d < %d", r.chars, st.minChars)
			}
			if r.confidence < st.minConf {
				t.Errorf("confidence too low: %d < %d", r.confidence, st.minConf)
			}
			for _, s := range st.mustContain {
				if !strings.Contains(r.contentPreview+"..."+r.title, s) {
					html := loadFixture(t, st.fixture)
					result := FromHTML(html, st.url)
					if !strings.Contains(result.Content, s) && !strings.Contains(result.Title, s) {
						t.Errorf("missing %q in content or title", s)
					}
				}
			}
			if st.mustNotSPA && r.isSPA {
				t.Error("should not be SPA")
			}
			if st.mustNotBlock && r.isBlocked {
				t.Error("should not be blocked")
			}

			t.Logf("  %s [%s]: %d chars | conf=%d | h=%d p=%d l=%d | q=%.0f | %dms | title=%q",
				r.name, r.category, r.chars, r.confidence, r.headings, r.paragraphs, r.links, r.quality, r.extractMs, truncStr(r.title, 50))
		})
	}
}

func TestExtended_QualityReport(t *testing.T) {
	t.Log("")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("                    CONTENT QUALITY REPORT")
	t.Log("═══════════════════════════════════════════════════════════════")

	for _, st := range extendedSites {
		r := extractFixture(t, st)

		structureIcon := "❌"
		if r.hasStructure {
			structureIcon = "✅"
		}
		linksIcon := "❌"
		if r.hasLinks {
			linksIcon = "✅"
		}

		t.Log("")
		t.Logf("── %s [%s] ──", r.name, r.category)
		t.Logf("   Title:      %q", truncStr(r.title, 80))
		t.Logf("   Size:       %d chars | Extract: %dms", r.chars, r.extractMs)
		t.Logf("   Confidence: %d%% | Quality: %.0f/100", r.confidence, r.quality)
		t.Logf("   Structure:  %s headings=%d paragraphs=%d | Links: %s count=%d",
			structureIcon, r.headings, r.paragraphs, linksIcon, r.links)
		if r.isSPA {
			t.Logf("   ⚠️  Classified as SPA")
		}
		if r.isBlocked {
			t.Logf("   🚫 Classified as BLOCKED")
		}
		t.Logf("   Preview:    %.150s", r.contentPreview)
	}
}

func TestExtended_PerformanceBenchmark(t *testing.T) {
	t.Log("")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("                   PERFORMANCE BENCHMARK")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("")

	type perfRow struct {
		name      string
		category  string
		htmlBytes int
		chars     int
		extractMs int64
		charPerMs float64
	}

	var rows []perfRow
	var totalMs int64

	for _, st := range extendedSites {
		fixture := st.fixture
		if st.mdFixture != "" {
			fixture = st.mdFixture
		}
		raw := loadFixture(t, fixture)
		htmlBytes := len(raw)

		var times []int64
		var lastChars int
		for i := 0; i < 3; i++ {
			if st.mdFixture != "" {
				start := time.Now()
				cleaned := CleanupMarkdown(raw)
				elapsed := time.Since(start).Milliseconds()
				times = append(times, elapsed)
				lastChars = len(cleaned)
			} else {
				start := time.Now()
				result := FromHTML(raw, st.url)
				elapsed := time.Since(start).Milliseconds()
				times = append(times, elapsed)
				lastChars = result.Length
			}
		}

		for i := 0; i < len(times)-1; i++ {
			for j := i + 1; j < len(times); j++ {
				if times[j] < times[i] {
					times[i], times[j] = times[j], times[i]
				}
			}
		}
		medianMs := times[len(times)/2]

		charPerMs := float64(0)
		if medianMs > 0 {
			charPerMs = float64(lastChars) / float64(medianMs)
		}

		rows = append(rows, perfRow{
			name:      st.name,
			category:  st.category,
			htmlBytes: htmlBytes,
			chars:     lastChars,
			extractMs: medianMs,
			charPerMs: charPerMs,
		})
		totalMs += medianMs
	}

	t.Logf("%-18s %-10s %10s %10s %8s %10s", "Site", "Category", "Input", "Output", "Time", "Throughput")
	t.Logf("%-18s %-10s %10s %10s %8s %10s", "────", "────────", "─────", "──────", "────", "──────────")
	for _, r := range rows {
		t.Logf("%-18s %-10s %10s %10s %6dms %8.0f c/ms",
			r.name, r.category, fmtBytes(r.htmlBytes), fmtChars(r.chars), r.extractMs, r.charPerMs)
	}
	t.Log("")
	t.Logf("Total extraction time: %dms across %d sites", totalMs, len(rows))
	t.Logf("Average: %dms per site", totalMs/int64(len(rows)))
}

func TestExtended_ComparisonTable(t *testing.T) {
	webFetchRef := map[string]int{
		"example.com":   1256,
		"hn":            10847,
		"wikipedia":     36429,
		"github-clay":   49229,
		"bbc":           827,
		"nytimes":       14570,
		"docs-openclaw": 7056,
		"react":         0,
		"cloudflare":    29508,
		"creepjs":       93,
		"stackoverflow": 22645,
		"medium":        8419,
		"reddit":        626,
		"python-docs":   35350,
		"mdn":           28908,
		"stripe-docs":   49229,
		"arxiv":         8337,
		"hn-item":       769,
	}

	t.Log("")
	t.Log("═══════════════════════════════════════════════════════════════════════════")
	t.Log("              SEAPORTAL vs WEB_FETCH COMPARISON (chars extracted)")
	t.Log("═══════════════════════════════════════════════════════════════════════════")
	t.Log("")
	t.Logf("%-18s %10s %10s %10s %-8s %s", "Site", "SeaPortal", "web_fetch", "Delta", "Winner", "Notes")
	t.Logf("%-18s %10s %10s %10s %-8s %s", "────", "─────────", "─────────", "─────", "──────", "─────")

	spWins, wfWins, ties, noData := 0, 0, 0, 0

	for _, st := range extendedSites {
		r := extractFixture(t, st)
		wf, hasRef := webFetchRef[st.name]

		spVal := r.chars
		notes := ""

		if st.mdFixture != "" {
			notes = "md-negotiation"
		}
		if r.isSPA {
			notes += " SPA"
		}
		if r.isBlocked {
			notes += " BLOCKED"
		}

		if !hasRef {
			t.Logf("%-18s %10d %10s %10s %-8s %s", st.name, spVal, "—", "—", "—", notes)
			noData++
			continue
		}

		delta := spVal - wf
		deltaStr := fmt.Sprintf("%+d", delta)
		winner := "TIE"
		if spVal > wf*12/10 {
			winner = "SP ✅"
			spWins++
		} else if wf > spVal*12/10 {
			winner = "wf ⚠️"
			wfWins++
		} else {
			ties++
		}

		t.Logf("%-18s %10d %10d %10s %-8s %s", st.name, spVal, wf, deltaStr, winner, notes)
	}

	t.Log("")
	t.Logf("Score: SeaPortal %d | web_fetch %d | Tie %d | No data %d", spWins, wfWins, ties, noData)
}

func truncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func fmtBytes(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fMB", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.0fKB", float64(n)/1_000)
	}
	return fmt.Sprintf("%dB", n)
}

func fmtChars(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
