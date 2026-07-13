package engine

import (
	"strings"
	"testing"

	pdfReader "github.com/ledongthuc/pdf"
)

// Word-runs separated by a gap wider than the kerning threshold get a space;
// an abutting run (e.g. trailing punctuation) does not.
func TestAssembleRow_InsertsSpacesOnWordGaps(t *testing.T) {
	runs := []pdfReader.Text{
		{S: "Hello", X: 0, W: 30, FontSize: 10},  // ends at 30
		{S: "world", X: 36, W: 30, FontSize: 10}, // gap 6 > 2 -> space
		{S: "!", X: 66, W: 3, FontSize: 10},      // gap 0 -> no space
	}
	if got := assembleRow(runs); got != "Hello world!" {
		t.Errorf("assembleRow = %q, want %q", got, "Hello world!")
	}
}

// Per-glyph runs that abut (the arxiv failure mode: LaTeX emits a run per glyph)
// must reassemble into a single word, not gain intra-word spaces.
func TestAssembleRow_NoSpaceWithinGluedGlyphs(t *testing.T) {
	runs := []pdfReader.Text{
		{S: "R", X: 0, W: 6, FontSize: 10},
		{S: "N", X: 6, W: 6, FontSize: 10},
		{S: "N", X: 12, W: 6, FontSize: 10},
	}
	if got := assembleRow(runs); got != "RNN" {
		t.Errorf("assembleRow = %q, want RNN", got)
	}
}

// An already-spaced run is not double-spaced.
func TestAssembleRow_NoDoubleSpace(t *testing.T) {
	runs := []pdfReader.Text{
		{S: "foo ", X: 0, W: 24, FontSize: 10},
		{S: "bar", X: 30, W: 18, FontSize: 10}, // gap 6 but prev ends in space
	}
	if got := assembleRow(runs); got != "foo bar" {
		t.Errorf("assembleRow = %q, want %q", got, "foo bar")
	}
}

// NFKC normalization removes the fi/fl/ff/ffi/ffl ligatures.
func TestNormalizePDFText_Ligatures(t *testing.T) {
	in := "eﬃcient ﬁgures ﬂow oﬀer" // ffi, fi, fl, ff
	got := normalizePDFText(in)
	if got != "efficient figures flow offer" {
		t.Errorf("normalizePDFText = %q, want %q", got, "efficient figures flow offer")
	}
	if strings.ContainsAny(got, "ﬀﬁﬂﬃﬄ") {
		t.Errorf("a U+FB0x ligature survived: %q", got)
	}
}
