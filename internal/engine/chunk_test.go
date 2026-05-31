package engine

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseChunkConfig_AllForms(t *testing.T) {
	cases := []struct {
		in   string
		want ChunkConfig
	}{
		{"", ChunkConfig{}},
		{"heading", ChunkConfig{Strategy: ChunkHeading}},
		{"sentence", ChunkConfig{Strategy: ChunkSentence, Size: 512}},
		{"sentence:256", ChunkConfig{Strategy: ChunkSentence, Size: 256}},
		{"window", ChunkConfig{Strategy: ChunkWindow, Size: 2000, Overlap: 200}},
		{"window:1000", ChunkConfig{Strategy: ChunkWindow, Size: 1000, Overlap: 0}},
		{"window:1000:100", ChunkConfig{Strategy: ChunkWindow, Size: 1000, Overlap: 100}},
	}
	for _, c := range cases {
		got, err := ParseChunkConfig(c.in)
		if err != nil {
			t.Errorf("ParseChunkConfig(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseChunkConfig(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestParseChunkConfig_DefaultsApplied(t *testing.T) {
	s, err := ParseChunkConfig("sentence")
	if err != nil || s.Size != 512 {
		t.Errorf("sentence default size: got %d err=%v, want 512", s.Size, err)
	}
	w, err := ParseChunkConfig("window")
	if err != nil || w.Size != 2000 || w.Overlap != 200 {
		t.Errorf("window defaults: got size=%d overlap=%d err=%v, want 2000/200", w.Size, w.Overlap, err)
	}
}

func TestParseChunkConfig_OverlapValidation(t *testing.T) {
	if _, err := ParseChunkConfig("window:100:100"); err == nil {
		t.Error("expected error for overlap==size")
	}
	if _, err := ParseChunkConfig("window:100:200"); err == nil {
		t.Error("expected error for overlap>size")
	}
}

func TestParseChunkConfig_UnknownNameError(t *testing.T) {
	if _, err := ParseChunkConfig("bogus"); err == nil {
		t.Error("expected error for unknown strategy")
	}
	if _, err := ParseChunkConfig("heading:5"); err == nil {
		t.Error("expected error for heading with params")
	}
}

func TestChunkByHeading_BasicSplit(t *testing.T) {
	md := "Intro paragraph here that is reasonably long for chunking purposes.\n\n" +
		"## Alpha\n\nAlpha body text content for the first section is here.\n\n" +
		"## Beta\n\nBeta body text content for the second section is here.\n\n" +
		"## Gamma\n\nGamma body text content for the third section is here.\n"
	chunks := ChunkMarkdown(md, ChunkConfig{Strategy: ChunkHeading})
	if len(chunks) != 4 {
		t.Fatalf("got %d chunks, want 4: %#v", len(chunks), chunks)
	}
	if chunks[0].Heading != "" || !strings.Contains(chunks[0].Text, "Intro") {
		t.Errorf("first chunk should be preamble with empty heading, got %+v", chunks[0])
	}
	for i, want := range []string{"## Alpha", "## Beta", "## Gamma"} {
		c := chunks[i+1]
		if c.Heading != want {
			t.Errorf("chunk %d heading = %q, want %q", i+1, c.Heading, want)
		}
		if !strings.HasPrefix(c.Text, want) {
			t.Errorf("chunk %d text should start with heading line: %q", i+1, c.Text)
		}
	}
}

// TestChunkByHeading_RecognisesH4 verifies that chunkByHeading splits on
// H4 (and by extension H5/H6) boundaries, not just H2/H3. Reference-style
// pages often nest per-entry subheadings (e.g. method descriptions, glossary
// terms) at H4+, and BM25 ranking benefits when those become their own chunks.
func TestChunkByHeading_RecognisesH4(t *testing.T) {
	md := "## A\n\nA body content goes here for the first section under H2.\n\n" +
		"#### B\n\nB body content under the H4 subheading for testing.\n"
	chunks := ChunkMarkdown(md, ChunkConfig{Strategy: ChunkHeading})
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks (## A and #### B), got %d: %+v", len(chunks), chunks)
	}
	if chunks[0].Heading != "## A" {
		t.Errorf("chunk 0 heading = %q, want %q", chunks[0].Heading, "## A")
	}
	if chunks[1].Heading != "#### B" {
		t.Errorf("chunk 1 heading = %q, want %q (H4 should be a section boundary)", chunks[1].Heading, "#### B")
	}
}

// TestChunkByHeading_RecognisesH5AndH6 sanity-checks that the broadened
// regex extends to the deepest heading levels.
func TestChunkByHeading_RecognisesH5AndH6(t *testing.T) {
	md := "## Top\n\nTop body content for the section is here.\n\n" +
		"##### Five\n\nFive body content goes here for testing purposes.\n\n" +
		"###### Six\n\nSix body content goes here for testing purposes.\n"
	chunks := ChunkMarkdown(md, ChunkConfig{Strategy: ChunkHeading})
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks, got %d: %+v", len(chunks), chunks)
	}
	wantHeadings := []string{"## Top", "##### Five", "###### Six"}
	for i, w := range wantHeadings {
		if chunks[i].Heading != w {
			t.Errorf("chunk %d heading = %q, want %q", i, chunks[i].Heading, w)
		}
	}
}

func TestChunkByHeading_PreservesCodeBlocks(t *testing.T) {
	md := "Intro that is long enough to trigger chunking by default.\n\n" +
		"## Real Heading\n\nBody text here.\n\n" +
		"```\n## Not a heading inside code\nmore code\n```\n\n" +
		"More body after code.\n"
	chunks := ChunkMarkdown(md, ChunkConfig{Strategy: ChunkHeading})
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 (intro + Real Heading), got: %#v", len(chunks), chunks)
	}
	if chunks[1].Heading != "## Real Heading" {
		t.Errorf("expected heading 'Real Heading', got %q", chunks[1].Heading)
	}
	if !strings.Contains(chunks[1].Text, "## Not a heading inside code") {
		t.Errorf("code block content lost: %q", chunks[1].Text)
	}
}

func TestChunkBySentence_GroupsToTarget(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "Sentence number %d here. ", i)
	}
	md := b.String()
	// size=50 tokens = 200 chars budget. 20 short sentences should produce >1 chunk.
	chunks := ChunkMarkdown(md, ChunkConfig{Strategy: ChunkSentence, Size: 50})
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d: %#v", len(chunks), chunks)
	}
	// Total text should be roughly preserved (modulo trimming/whitespace).
	var total int
	for _, c := range chunks {
		total += len(c.Text)
	}
	if total < len(strings.TrimSpace(md))/2 {
		t.Errorf("chunks lost too much content: total=%d original=%d", total, len(md))
	}
}

func TestChunkBySentence_HeadingInheritance(t *testing.T) {
	md := "## Section One\n\n" +
		"Alpha sentence one. Alpha sentence two. Alpha sentence three. Alpha sentence four.\n\n" +
		"## Section Two\n\n" +
		"Beta sentence one. Beta sentence two. Beta sentence three. Beta sentence four.\n"
	chunks := ChunkMarkdown(md, ChunkConfig{Strategy: ChunkSentence, Size: 30})
	if len(chunks) == 0 {
		t.Fatal("no chunks produced")
	}
	sawOne, sawTwo := false, false
	for _, c := range chunks {
		if c.Heading == "## Section One" {
			sawOne = true
		}
		if c.Heading == "## Section Two" {
			sawTwo = true
		}
	}
	if !sawOne || !sawTwo {
		t.Errorf("heading inheritance missing: sawOne=%v sawTwo=%v chunks=%+v", sawOne, sawTwo, chunks)
	}
}

func TestChunkByWindow_OverlapBackstep(t *testing.T) {
	// 600 chars of plain text, window=200, overlap=50 → step=150.
	md := strings.Repeat("abcdefghij ", 60) // 660 chars
	chunks := ChunkMarkdown(md, ChunkConfig{Strategy: ChunkWindow, Size: 200, Overlap: 50})
	if len(chunks) < 2 {
		t.Fatalf("expected multiple windows, got %d", len(chunks))
	}
	// Adjacent chunks should share a tail/head suffix close to overlap chars
	// (allowing for word-boundary snap reducing it slightly).
	for i := 0; i+1 < len(chunks); i++ {
		a, b := chunks[i].Text, chunks[i+1].Text
		// Find max k <= 50 such that suffix of a == prefix of b.
		maxK := 50
		if len(a) < maxK {
			maxK = len(a)
		}
		if len(b) < maxK {
			maxK = len(b)
		}
		found := 0
		for k := maxK; k >= 1; k-- {
			if strings.HasSuffix(a, b[:k]) {
				found = k
				break
			}
		}
		if found < 20 {
			t.Errorf("chunks %d/%d: overlap=%d chars, expected at least ~20", i, i+1, found)
		}
		if found > 50 {
			t.Errorf("chunks %d/%d: overlap=%d > cfg.Overlap=50", i, i+1, found)
		}
	}
}

func TestChunkByWindow_DoesNotBreakMidWord(t *testing.T) {
	// All lowercase words separated by spaces; verify each chunk ends at a
	// complete word boundary. (Chunks are trimmed, so "ends at a space" means
	// the chunk text terminates with a complete word — the next char in the
	// source is whitespace.)
	md := strings.Repeat("alphabet bravo charlie delta echo foxtrot ", 30)
	knownWords := []string{"alphabet", "bravo", "charlie", "delta", "echo", "foxtrot"}
	chunks := ChunkMarkdown(md, ChunkConfig{Strategy: ChunkWindow, Size: 200, Overlap: 0})
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks[:len(chunks)-1] {
		// The chunk's final word must be a complete known word (not a prefix).
		fields := strings.Fields(c.Text)
		if len(fields) == 0 {
			continue
		}
		last := fields[len(fields)-1]
		matched := false
		for _, w := range knownWords {
			if last == w {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("chunk %d ends mid-word with %q (tail %q)", i, last, c.Text[max(0, len(c.Text)-20):])
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Integration tests via httptest ──────────────────────────────────

func chunkTestServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
}

const chunkSyntheticHTML = `<!DOCTYPE html>
<html><head><title>Chunking Sample</title></head><body>
<article>
<h1>Chunking Sample Article</h1>
<p>This is the introductory paragraph for the chunking sample article we use in integration tests, with enough text to clear the readability bar.</p>
<h2>Alpha</h2>
<p>This is the alpha section body text. It contains several sentences. Each one is short. They appear in document order. The alpha section is moderately long for testing.</p>
<h2>Beta</h2>
<p>This is the beta section body text. It also has several sentences. They are present here. The beta section similarly contains some prose. Beta beta beta.</p>
<h2>Gamma</h2>
<p>This is the gamma section body text. Gamma has its own sentences too. They continue. Some more gamma content. Gamma gamma gamma. Final gamma sentence.</p>
</article>
</body></html>`

func TestExtract_ChunkFlagOn_Heading(t *testing.T) {
	srv := chunkTestServer(t, chunkSyntheticHTML)
	defer srv.Close()
	cfg, _ := ParseChunkConfig("heading")
	res := FromURLWithOptions(srv.URL, Options{Chunk: cfg})
	if len(res.Chunks) == 0 {
		t.Fatalf("expected chunks, got none; content=%q", res.Content)
	}
	if len(res.Chunks) < 3 {
		t.Errorf("expected at least 3 chunks (one per heading), got %d", len(res.Chunks))
	}
	headings := 0
	for _, c := range res.Chunks {
		if c.Heading != "" {
			headings++
		}
	}
	if headings == 0 {
		t.Errorf("expected at least one chunk with a heading, got %+v", res.Chunks)
	}
}

func TestExtract_ChunkFlagOn_Sentence(t *testing.T) {
	srv := chunkTestServer(t, chunkSyntheticHTML)
	defer srv.Close()
	cfg, _ := ParseChunkConfig("sentence:50")
	res := FromURLWithOptions(srv.URL, Options{Chunk: cfg})
	if len(res.Chunks) == 0 {
		t.Fatalf("expected chunks, got none; content=%q", res.Content)
	}
	for i, c := range res.Chunks {
		if c.Index != i {
			t.Errorf("chunk %d index = %d, want %d", i, c.Index, i)
		}
		if c.Text == "" {
			t.Errorf("chunk %d has empty text", i)
		}
	}
}

func TestExtract_ChunkFlagOn_Window(t *testing.T) {
	srv := chunkTestServer(t, chunkSyntheticHTML)
	defer srv.Close()
	cfg, _ := ParseChunkConfig("window:200:50")
	res := FromURLWithOptions(srv.URL, Options{Chunk: cfg})
	if len(res.Chunks) == 0 {
		t.Fatalf("expected chunks, got none; content=%q", res.Content)
	}
	for i, c := range res.Chunks {
		if c.Tokens <= 0 && len(c.Text) >= charsPerToken {
			t.Errorf("chunk %d tokens=%d, expected >0 for text len=%d", i, c.Tokens, len(c.Text))
		}
	}
}

func TestExtract_ChunkFlagOffDefault(t *testing.T) {
	srv := chunkTestServer(t, chunkSyntheticHTML)
	defer srv.Close()
	res := FromURLWithOptions(srv.URL, Options{})
	if res.Chunks != nil {
		t.Errorf("expected nil Chunks by default, got %d: %+v", len(res.Chunks), res.Chunks)
	}
}

// TestSoftSplitChunk_BoldParagraphBoundaries verifies that an oversized chunk
// containing exclusive-bold lines (e.g. "**Term**") is fragmented into one
// sub-chunk per bold boundary, with headings of the form "<parent> · <term>".
func TestSoftSplitChunk_BoldParagraphBoundaries(t *testing.T) {
	// Build a >600-char chunk with two bold-prefixed entries.
	filler := strings.Repeat("Lorem ipsum dolor sit amet consectetur adipiscing elit. ", 6)
	body := "## Glossary\nIntro paragraph here.\n" +
		"**Alpha**\n" + filler + "\n" +
		"**Beta**\n" + filler + "\n"
	parent := rawChunk{heading: "## Glossary", text: body}
	got := softSplitChunk(parent, softSplitThresholdChars)
	if len(got) < 2 {
		t.Fatalf("expected >=2 sub-chunks, got %d: %+v", len(got), got)
	}
	var foundAlpha, foundBeta bool
	for _, c := range got {
		if strings.Contains(c.heading, "· Alpha") {
			foundAlpha = true
		}
		if strings.Contains(c.heading, "· Beta") {
			foundBeta = true
		}
	}
	if !foundAlpha || !foundBeta {
		t.Errorf("expected sub-chunks for both Alpha and Beta; got headings:\n%s", dumpHeadings(got))
	}
}

// TestSoftSplitChunk_TableRowBoundaries verifies that an oversized chunk
// containing Markdown table rows is fragmented per-row, with the separator
// row swallowed by its preceding row (NOT a sub-chunk on its own).
func TestSoftSplitChunk_TableRowBoundaries(t *testing.T) {
	pad := strings.Repeat("padding ", 80) // push us over threshold
	body := "## Methods\n" + pad + "\n" +
		"| Method | Description |\n" +
		"|--------|-------------|\n" +
		"| DELETE | removes a resource |\n" +
		"| GET    | retrieves a resource |\n"
	parent := rawChunk{heading: "## Methods", text: body}
	got := softSplitChunk(parent, softSplitThresholdChars)
	if len(got) < 3 {
		t.Fatalf("expected >=3 sub-chunks (preamble + header row + 2 data rows or similar); got %d:\n%s",
			len(got), dumpHeadings(got))
	}
	var foundDelete, foundGet bool
	for _, c := range got {
		if strings.Contains(c.heading, "· DELETE") {
			foundDelete = true
		}
		if strings.Contains(c.heading, "· GET") {
			foundGet = true
		}
		// Separator row must NEVER become its own boundary key.
		if strings.Contains(c.heading, "---") {
			t.Errorf("separator row leaked into heading: %q", c.heading)
		}
	}
	if !foundDelete || !foundGet {
		t.Errorf("expected per-row sub-chunks for DELETE and GET; got:\n%s", dumpHeadings(got))
	}
}

// TestSoftSplitChunk_SkipsBelowThreshold verifies tiny chunks pass through
// unchanged even if they contain boundary-shaped lines.
func TestSoftSplitChunk_SkipsBelowThreshold(t *testing.T) {
	body := "## Tiny\n**Alpha**\nbody\n**Beta**\nmore\n"
	parent := rawChunk{heading: "## Tiny", text: body}
	got := softSplitChunk(parent, softSplitThresholdChars)
	if len(got) != 1 || got[0].text != body {
		t.Errorf("expected 1 unchanged chunk for below-threshold input; got %d:\n%s", len(got), dumpHeadings(got))
	}
}

// TestSoftSplitChunk_SkipsFencedCode verifies bold-looking and table-looking
// content inside a fenced code block does NOT trigger a soft-split.
func TestSoftSplitChunk_SkipsFencedCode(t *testing.T) {
	pad := strings.Repeat("padding ", 80)
	body := "## Code\n" + pad + "\n" +
		"```\n" +
		"**not_a_boundary**\n" +
		"| also | not | a | row |\n" +
		"|------|-----|---|-----|\n" +
		"```\n" +
		"trailing prose\n"
	parent := rawChunk{heading: "## Code", text: body}
	got := softSplitChunk(parent, softSplitThresholdChars)
	if len(got) != 1 {
		t.Errorf("expected 1 chunk (no boundaries outside code fence); got %d:\n%s", len(got), dumpHeadings(got))
	}
}

func dumpHeadings(cs []rawChunk) string {
	var sb strings.Builder
	for i, c := range cs {
		fmt.Fprintf(&sb, "  %d. heading=%q text=%q\n", i, c.heading, c.text)
	}
	return sb.String()
}
