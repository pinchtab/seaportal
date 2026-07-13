package engine

import (
	"fmt"
	"strings"
	"testing"
)

func TestBM25Quality_MDNHTTPMethods_DeleteQueryRanksDeleteSection(t *testing.T) {
	html := loadFixture(t, "ssr/mdn-http-methods.html")
	r := FromHTMLWithOptions(html, "https://example.com/methods",
		Options{Query: "DELETE method semantics", TopN: 5})
	if r.Error != "" {
		t.Fatalf("extraction error: %s", r.Error)
	}
	if len(r.RankedSections) == 0 {
		t.Fatalf("no sections ranked; Content=%q", r.Content)
	}
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

func TestBM25Quality_WikipediaLatinPhrases_CarpeDiemRanksCSection(t *testing.T) {
	skipHeavyFixture(t)
	html := loadFixture(t, "static/wikipedia-latin-phrases.html")
	r := FromHTMLWithOptions(html, "https://example.com/latin",
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
