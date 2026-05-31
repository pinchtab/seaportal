package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- scopeMainContent unit tests ----

func TestScopeMainContent_PrefersMain(t *testing.T) {
	in := `<html><body>
<article><h1>Article block</h1><p>article body</p></article>
<main><h1>Main block</h1><p>main body that is the real content</p></main>
</body></html>`
	out := scopeMainContent(in)
	if !strings.Contains(out, "Main block") {
		t.Fatalf("expected main content present:\n%s", out)
	}
	if strings.Contains(out, "Article block") {
		t.Errorf("expected article block stripped when main present:\n%s", out)
	}
}

func TestScopeMainContent_FallsBackToArticle(t *testing.T) {
	in := `<html><body>
<div><p>chrome-ish lead</p></div>
<article><h1>Article wins</h1><p>` + strings.Repeat("Lorem ipsum dolor sit amet. ", 20) + `</p></article>
</body></html>`
	out := scopeMainContent(in)
	if !strings.Contains(out, "Article wins") {
		t.Fatalf("expected article picked, got:\n%s", out)
	}
}

func TestScopeMainContent_LargestDivFallback(t *testing.T) {
	big := strings.Repeat("This is the largest container with ample text content. ", 10)
	in := `<html><body><div><div><p>tiny</p></div><div><p>medium small</p></div><div><p>` + big + `</p></div></div></body></html>`
	out := scopeMainContent(in)
	if !strings.Contains(out, "largest container") {
		t.Fatalf("expected largest-div pick to win, got:\n%s", out)
	}
	if strings.Contains(out, "<p>tiny</p>") {
		t.Errorf("expected unrelated small div pruned:\n%s", out)
	}
}

func TestScopeMainContent_NoOpWhenNoMatch(t *testing.T) {
	in := `<html><body><p>just a single paragraph</p><p>and another</p></body></html>`
	out := scopeMainContent(in)
	// Renderer may normalise whitespace, but the body content semantics
	// should be untouched: no <article> wrapper added, paragraphs remain.
	if strings.Contains(out, "<article>") {
		t.Errorf("expected no <article> wrapper for flat-DOM no-anchor case, got:\n%s", out)
	}
	if !strings.Contains(out, "just a single paragraph") {
		t.Errorf("expected original paragraph preserved:\n%s", out)
	}
}

// ---- stripCommonChrome unit tests ----

func TestStripCommonChrome_RemovesNavAside(t *testing.T) {
	in := `<html><body>
<header><nav>top nav</nav></header>
<nav>side nav</nav>
<aside>side widget</aside>
<main><p>real content</p></main>
<footer>footer text</footer>
<div class="sidebar">sidebar text</div>
</body></html>`
	out := stripCommonChrome(in)
	for _, want := range []string{"real content"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q preserved, got:\n%s", want, out)
		}
	}
	for _, gone := range []string{"top nav", "side nav", "side widget", "footer text", "sidebar text"} {
		if strings.Contains(out, gone) {
			t.Errorf("expected %q stripped:\n%s", gone, out)
		}
	}
}

func TestStripCommonChrome_KeepsBareHeaderWithoutNav(t *testing.T) {
	in := `<html><body><header><h1>Article Title</h1></header><p>body text</p></body></html>`
	out := stripCommonChrome(in)
	if !strings.Contains(out, "Article Title") {
		t.Errorf("expected bare-header title preserved:\n%s", out)
	}
}

func TestStripCommonChrome_StripsByRole(t *testing.T) {
	in := `<html><body>
<div role="banner">brand</div>
<div role="navigation">links</div>
<div role="contentinfo">copyright</div>
<p>body text</p>
</body></html>`
	out := stripCommonChrome(in)
	if !strings.Contains(out, "body text") {
		t.Errorf("expected body text preserved:\n%s", out)
	}
	for _, gone := range []string{"brand", "links", "copyright"} {
		if strings.Contains(out, gone) {
			t.Errorf("expected %q stripped by role:\n%s", gone, out)
		}
	}
}

func TestStripCommonChrome_StripsCookieBanners(t *testing.T) {
	in := `<html><body>
<div aria-label="Cookie consent banner">accept all</div>
<p>body text</p>
</body></html>`
	out := stripCommonChrome(in)
	if strings.Contains(out, "accept all") {
		t.Errorf("expected cookie banner stripped:\n%s", out)
	}
	if !strings.Contains(out, "body text") {
		t.Errorf("expected body text preserved:\n%s", out)
	}
}

// ---- stripHighLinkDensityBlocks unit tests ----

func TestStripHighLinkDensity_TagCloud(t *testing.T) {
	prose := strings.Repeat("This is genuine article prose that anchors the page and gives the body real content beyond the tag cloud. ", 4)
	var sb strings.Builder
	sb.WriteString(`<html><body><main><article><p>` + prose + `</p></article><div id="tagcloud">`)
	words := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta", "iota", "kappa", "lambda", "mu", "nu", "xi", "omicron", "pi", "rho", "sigma", "tau", "upsilon"}
	for _, w := range words {
		sb.WriteString(`<a href="/tag/` + w + `">` + w + `</a> `)
	}
	sb.WriteString(`</div></main></body></html>`)
	out := stripHighLinkDensityBlocks(sb.String())
	if strings.Contains(out, `id="tagcloud"`) {
		t.Errorf("expected tag cloud removed:\n%s", out)
	}
	if !strings.Contains(out, "genuine article prose") {
		t.Errorf("expected article prose preserved:\n%s", out)
	}
}

func TestStripHighLinkDensity_RelatedArticlesWidget(t *testing.T) {
	prose := strings.Repeat("Main article prose stays put and gives the body enough real content to keep the related widget from looking like the page's only material. ", 3)
	in := `<html><body><main><article><p>` + prose + `</p></article>
<section class="widget-related"><h3>Related</h3><ul>
<li><a href="/a1">First article title goes here</a></li>
<li><a href="/a2">Second article title goes here</a></li>
<li><a href="/a3">Third article title goes here</a></li>
<li><a href="/a4">Fourth article title goes here</a></li>
<li><a href="/a5">Fifth article title goes here</a></li>
</ul></section></main></body></html>`
	out := stripHighLinkDensityBlocks(in)
	if strings.Contains(out, "Fifth article title") {
		t.Errorf("expected related-articles widget removed:\n%s", out)
	}
	if !strings.Contains(out, "Main article prose") {
		t.Errorf("expected main prose preserved:\n%s", out)
	}
}

func TestStripHighLinkDensity_KeepsParagraphWithCitations(t *testing.T) {
	prose := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 5) // ~225 chars
	in := `<html><body><main><div class="article-body"><p>` + prose +
		`See <a href="/r1">ref</a>, <a href="/r2">two</a>, and <a href="/r3">three</a>.</p></div></main></body></html>`
	out := stripHighLinkDensityBlocks(in)
	if !strings.Contains(out, "quick brown fox") {
		t.Errorf("expected paragraph with citations preserved:\n%s", out)
	}
	if !strings.Contains(out, `class="article-body"`) {
		t.Errorf("expected outer div preserved:\n%s", out)
	}
}

func TestStripHighLinkDensity_KeepsShortBlocks(t *testing.T) {
	in := `<html><body><main><p>prose</p><div class="mini"><a href="/x">x</a> <a href="/y">y</a> <a href="/z">z</a></div></main></body></html>`
	out := stripHighLinkDensityBlocks(in)
	if !strings.Contains(out, `class="mini"`) {
		t.Errorf("expected short block (<50 chars) preserved:\n%s", out)
	}
}

func TestStripHighLinkDensity_KeepsLowAnchorCount(t *testing.T) {
	prose := strings.Repeat("Lorem ipsum dolor sit amet consectetur adipiscing elit. ", 4) // ~225 chars
	in := `<html><body><main><div class="cite-block">` + prose +
		`See <a href="/one">first</a> and <a href="/two">second</a>.</div></main></body></html>`
	out := stripHighLinkDensityBlocks(in)
	if !strings.Contains(out, `class="cite-block"`) {
		t.Errorf("expected low-anchor-count block preserved:\n%s", out)
	}
}

// ---- preprocess baseline matrix (integration) ----

func TestPreprocessBaseline_ExtractionMatrix(t *testing.T) {
	for _, row := range preprocessBaseline {
		row := row
		t.Run(row.fixture, func(t *testing.T) {
			// The Wikipedia Latin phrases fixture is a 1.4 MB HTML page; under
			// `-race -coverprofile` it can run for several minutes, blowing
			// past the default 10m test timeout. The same regression contract
			// is already enforced by TestExtract_WikipediaLatinPhrases in
			// extract_wikipedia_test.go (length > 20k + content markers), so
			// we skip the duplicate here under -short or race.
			if (testing.Short() || isRaceEnabled) && strings.Contains(row.fixture, "wikipedia-latin") {
				t.Skipf("skipping heavy fixture under short/race; covered by TestExtract_WikipediaLatinPhrases")
			}
			// Search testdata/ + known class subfolders for the bare name.
			// Lets the baseline matrix keep bare fixture names after the
			// 2026-05-17 reorg moved real-world fixtures into class folders.
			var data []byte
			var lastErr error
			for _, sub := range []string{"", "static", "ssr", "dynamic", "hydrated", "blocked", "multilingual"} {
				p := filepath.Join("..", "..", "testdata", sub, row.fixture)
				if d, err := os.ReadFile(p); err == nil {
					data = d
					break
				} else {
					lastErr = err
				}
			}
			if data == nil {
				t.Skipf("fixture %s not present (last error: %v)", row.fixture, lastErr)
			}
			r := FromHTML(string(data), row.url)
			if r.Error != "" {
				t.Fatalf("extraction error: %s", r.Error)
			}
			if row.minLength > 0 && r.Length < row.minLength {
				t.Errorf("REGRESSION: %s length=%d, baseline=%d, min=%d",
					row.fixture, r.Length, row.preLength, row.minLength)
			}
			lower := strings.ToLower(r.Content + " " + r.Title)
			for _, m := range row.markers {
				if !strings.Contains(lower, strings.ToLower(m)) {
					t.Errorf("MARKER MISSING in %s: %q", row.fixture, m)
				}
			}
			t.Logf("%s len=%d (baseline=%d, %+d) %s",
				row.fixture, r.Length, row.preLength, r.Length-row.preLength, row.acceptedDelta)
		})
	}
}
