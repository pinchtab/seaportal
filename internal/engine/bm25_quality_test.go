package engine

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestBM25Quality_MDNHTTPMethods_DeleteQueryRanksDeleteSection exercises the
// full FromHTMLWithOptions pipeline (extraction → markdown → BM25 ranking)
// against the MDN HTTP methods fixture for a query about the DELETE method.
//
// MDN packs every method (GET/POST/DELETE/...) as a single <table> under a
// single H2. There are no per-method H2/H3 anchors. The chunker's heading
// pass alone therefore cannot surface a DELETE-specific chunk; we rely on
// the soft-split second pass (table-row boundaries) to emit one sub-chunk
// per method, headed "<parent> · DELETE".
//
// HISTORY:
//   - 2026-05-17 (initial lock-in): pre-soft-split. Top-3 was
//     1. "## Specifications"               (~3.52)
//     2. "## Safe, idempotent..."          (~1.34, actually mentions DELETE)
//     3. "## Browser compatibility"        (0)
//     Test asserted top-3 contained "DELETE" anywhere in body text and that
//     #2 was the "Safe..." section.
//   - 2026-05-17 (soft-split landing): bold-paragraph / table-row soft splits
//     added in chunk.go. New top-3 for "DELETE method semantics":
//     1. "## Specifications"                              (~5.14) ← URL slug match
//     2. "## Safe, idempotent... · Method"                (~3.65) ← table header row
//     3. "## Safe, idempotent... · DELETE"                (~2.86) ← per-method row
//     DELETE no longer #1 (the Specifications section out-scores it because
//     the slug literally contains "DELETE" four times via per-method
//     spec-links). But a chunk whose heading is literally "...· DELETE" now
//     exists and ranks top-3 — assert exactly that.
func TestBM25Quality_MDNHTTPMethods_DeleteQueryRanksDeleteSection(t *testing.T) {
	html, err := os.ReadFile("../../testdata/ssr/mdn-http-methods.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	r := FromHTMLWithOptions(string(html), "https://example.com/methods",
		Options{Query: "DELETE method semantics", TopN: 5})
	if r.Error != "" {
		t.Fatalf("extraction error: %s", r.Error)
	}
	if len(r.RankedSections) == 0 {
		t.Fatalf("no sections ranked; Content=%q", r.Content)
	}
	// Tightened assertion (post soft-split): a chunk whose HEADING contains
	// "DELETE" — i.e. the per-method sub-chunk produced by the table-row
	// soft-splitter — must appear in the top-3.
	foundDeleteHeadingInTop3 := false
	for i := 0; i < len(r.RankedSections) && i < 3; i++ {
		if strings.Contains(strings.ToUpper(r.RankedSections[i].Heading), "DELETE") {
			foundDeleteHeadingInTop3 = true
			break
		}
	}
	if !foundDeleteHeadingInTop3 {
		t.Errorf("no top-3 section HEADING contains DELETE; soft-split per-method chunk did not surface.\nTop-5:\n%s",
			dumpRankedHeadings(r.RankedSections, 5))
	}
}

// TestBM25Quality_WikipediaLatinPhrases_CarpeDiemRanksCSection exercises the
// pipeline against the (large) Wikipedia Latin phrases fixture. The phrase
// "carpe diem" appears in the C-prefixed section of the glossary; we expect
// a query for it to surface a section that actually contains the phrase in
// the top-3 results.
func TestBM25Quality_WikipediaLatinPhrases_CarpeDiemRanksCSection(t *testing.T) {
	html, err := os.ReadFile("../../testdata/static/wikipedia-latin-phrases.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	r := FromHTMLWithOptions(string(html), "https://example.com/latin",
		Options{Query: "carpe diem", TopN: 3})
	if r.Error != "" {
		t.Fatalf("extraction error: %s", r.Error)
	}
	if len(r.RankedSections) == 0 {
		t.Fatalf("no sections ranked; Content length=%d", len(r.Content))
	}
	foundInTop := false
	for i := 0; i < len(r.RankedSections) && i < 3; i++ {
		if strings.Contains(strings.ToLower(r.RankedSections[i].Text), "carpe diem") {
			foundInTop = true
			break
		}
	}
	if !foundInTop {
		t.Errorf("carpe diem not in any top-3 section text.\nTop-3:\n%s",
			dumpRankedHeadings(r.RankedSections, 3))
	}
	if testing.Verbose() {
		t.Logf("wikipedia-latin-phrases top-3 for \"carpe diem\":\n%s",
			dumpRankedHeadings(r.RankedSections, 3))
	}
}

func dumpRankedHeadings(rs []RankedSection, n int) string {
	if n > len(rs) {
		n = len(rs)
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "  %d. score=%.4f tokens=%d heading=%q\n",
			i+1, rs[i].Score, rs[i].Tokens, rs[i].Heading)
	}
	return sb.String()
}
