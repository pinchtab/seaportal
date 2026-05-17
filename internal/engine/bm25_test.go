package engine

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRankSections_QueryBoostsRelevantSection(t *testing.T) {
	content := `## Alpha
The alpha section talks about apples and bananas. Fruit grows on trees.

## Beta
The beta section is about compound interest. Compound interest formulas and
compound interest examples appear repeatedly in this beta section.

## Gamma
The gamma section discusses geometry, angles, and triangles.`
	ranked := RankSections(content, "compound interest", 0, 0, 0)
	if len(ranked) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(ranked))
	}
	if !strings.Contains(ranked[0].Heading, "Beta") {
		t.Errorf("expected Beta to rank first, got %q (score=%v)", ranked[0].Heading, ranked[0].Score)
	}
	if ranked[0].Score <= ranked[1].Score {
		t.Errorf("expected strictly descending: %v vs %v", ranked[0].Score, ranked[1].Score)
	}
}

func TestRankSections_TopNTruncates(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 5; i++ {
		sb.WriteString("## Section ")
		sb.WriteByte(byte('A' + i))
		sb.WriteString("\nThis section talks about widgets and gadgets sub")
		sb.WriteByte(byte('A' + i))
		sb.WriteString(".\n\n")
	}
	ranked := RankSections(sb.String(), "widgets", 0, 0, 2)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 sections after top-N, got %d", len(ranked))
	}
}

func TestRankSections_EmptyQueryReturnsNil(t *testing.T) {
	if got := RankSections("## A\nhello world", "", 0, 0, 0); got != nil {
		t.Errorf("expected nil for empty query, got %+v", got)
	}
	if got := RankSections("## A\nhello world", "   ", 0, 0, 0); got != nil {
		t.Errorf("expected nil for whitespace query, got %+v", got)
	}
}

func TestRankSections_NoHeadingsSingleSection(t *testing.T) {
	content := "This document has no headings. It only contains some prose about kangaroos and koalas living together in Australia."
	ranked := RankSections(content, "kangaroos", 0, 0, 0)
	if len(ranked) != 1 {
		t.Fatalf("expected 1 section for heading-less doc, got %d", len(ranked))
	}
	if ranked[0].Score <= 0 {
		t.Errorf("expected positive score for matching term, got %v", ranked[0].Score)
	}
	if ranked[0].Heading != "" {
		t.Errorf("expected empty heading, got %q", ranked[0].Heading)
	}
}

func TestRankSections_LengthNormalisation(t *testing.T) {
	// Two sections with identical query term counts but very different lengths.
	// BM25 length normalisation should score the shorter section higher.
	short := "## Short\nrelevant relevant"
	long := "## Long\nrelevant relevant " + strings.Repeat("filler words here ", 80)
	content := short + "\n\n" + long
	ranked := RankSections(content, "relevant", 0, 0, 0)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(ranked))
	}
	if !strings.Contains(ranked[0].Heading, "Short") {
		t.Errorf("expected Short to outrank Long, got %q first", ranked[0].Heading)
	}
}

func TestRankSections_ZeroMatchScoresZero(t *testing.T) {
	content := "## A\napples\n\n## B\nbananas\n\n## C\ncherries"
	ranked := RankSections(content, "zzzzz", 0, 0, 0)
	if len(ranked) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(ranked))
	}
	for _, r := range ranked {
		if r.Score != 0 {
			t.Errorf("expected score 0 for no-match term, got %v in %q", r.Score, r.Heading)
		}
	}
}

func TestRankSections_BM25MathHandComputed(t *testing.T) {
	// Two sections, each one token, query = "x". Section A = "x", Section B = "y".
	// N=2, df(x)=1, dl_A=1, dl_B=1, avgdl=1.
	// idf = ln((2-1+0.5)/(1+0.5) + 1) = ln(1 + 1) = ln(2)
	// tfNorm_A = 1*(k1+1) / (1 + k1*(1-b+b*1/1)) = (k1+1)/(1+k1)
	// score_A = ln(2) * (k1+1)/(k1+1) = ln(2).
	ranked := RankSections("## A\nx\n\n## B\ny", "x", 1.5, 0.75, 0)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(ranked))
	}
	want := math.Log(2)
	if math.Abs(ranked[0].Score-want) > 1e-9 {
		t.Errorf("expected score=%v, got %v", want, ranked[0].Score)
	}
	if ranked[1].Score != 0 {
		t.Errorf("expected B score=0, got %v", ranked[1].Score)
	}
}

// ── Integration tests via httptest ──────────────────────────────────

const bm25SyntheticHTML = `<!DOCTYPE html>
<html><head><title>BM25 Sample</title></head><body>
<article>
<h1>BM25 Sample Article</h1>
<p>This is the introductory paragraph for the BM25 ranking sample article we use in integration tests, with enough text to clear the readability bar.</p>
<h2>Apples</h2>
<p>This is the apples section. Apples are red fruit. Apples grow on trees. People eat apples. The apples section is moderately long for testing.</p>
<h2>Compound Interest</h2>
<p>This is the compound interest section. Compound interest is interest on interest. The compound interest formula is A=P(1+r/n)^(nt). Compound interest matters for savings.</p>
<h2>Geometry</h2>
<p>This is the geometry section. Geometry studies shapes and angles. Triangles, circles, and squares. Geometry has its own vocabulary too.</p>
</article>
</body></html>`

func bm25TestServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
}

func TestExtract_QueryAnnotation(t *testing.T) {
	srv := bm25TestServer(t, bm25SyntheticHTML)
	defer srv.Close()
	res := FromURLWithOptions(srv.URL, Options{Query: "compound interest"})
	if len(res.RankedSections) == 0 {
		t.Fatalf("expected RankedSections to be populated; content=%q", res.Content)
	}
	// Descending order check.
	for i := 1; i < len(res.RankedSections); i++ {
		if res.RankedSections[i].Score > res.RankedSections[i-1].Score {
			t.Errorf("RankedSections not in descending order at %d: %v > %v",
				i, res.RankedSections[i].Score, res.RankedSections[i-1].Score)
		}
	}
	// "Compound Interest" section should win.
	top := res.RankedSections[0]
	if !strings.Contains(strings.ToLower(top.Heading+" "+top.Text), "compound interest") {
		t.Errorf("expected top section to be the compound interest one, got heading=%q", top.Heading)
	}
	// Content untouched in annotation-only mode: must still contain the geometry section.
	if !strings.Contains(strings.ToLower(res.Content), "geometry") {
		t.Errorf("expected Content untouched (still contains 'geometry'), got: %q", res.Content)
	}
}

func TestExtract_FilterByQueryReplacesContent(t *testing.T) {
	srv := bm25TestServer(t, bm25SyntheticHTML)
	defer srv.Close()
	res := FromURLWithOptions(srv.URL, Options{
		Query:         "compound interest",
		TopN:          2,
		FilterByQuery: true,
	})
	if len(res.RankedSections) == 0 {
		t.Fatalf("expected RankedSections to be populated")
	}
	if len(res.RankedSections) != 2 {
		t.Errorf("expected RankedSections truncated to TopN=2, got %d", len(res.RankedSections))
	}
	lower := strings.ToLower(res.Content)
	if !strings.Contains(lower, "compound interest") {
		t.Errorf("expected filtered Content to contain top section, got: %q", res.Content)
	}
	// Geometry should be dropped when topN=2 and it's the lowest-scored.
	if strings.Contains(lower, "geometry") {
		t.Errorf("expected geometry section dropped from filtered Content, got: %q", res.Content)
	}
	if res.Length != len(res.Content) {
		t.Errorf("Length=%d but len(Content)=%d", res.Length, len(res.Content))
	}
}
