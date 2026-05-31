package engine

import (
	"strings"
	"testing"
)

func TestDedupeBasic(t *testing.T) {
	content := `# Welcome

This is the main content.

# Navigation

Home | About | Contact

# Main Article

This is the main content.

# Navigation

Home | About | Contact`

	result := Dedupe(content)

	if result.DuplicatesFound != 3 {
		t.Errorf("Expected 3 duplicates, got %d", result.DuplicatesFound)
	}

	if strings.Count(result.Content, "This is the main content") > 1 {
		t.Error("Duplicate content not removed")
	}

	if strings.Count(result.Content, "Home | About | Contact") > 1 {
		t.Error("Duplicate nav not removed")
	}
}

func TestDedupePreservesOrder(t *testing.T) {
	content := `# First

First paragraph.

# Second

Second paragraph.

# First

First paragraph.`

	result := Dedupe(content)

	if !strings.Contains(result.Content, "# First") {
		t.Error("First heading should be preserved")
	}
	if !strings.Contains(result.Content, "First paragraph") {
		t.Error("First paragraph should be preserved")
	}

	firstIdx := strings.Index(result.Content, "# First")
	secondIdx := strings.Index(result.Content, "# Second")

	if firstIdx > secondIdx {
		t.Error("Order not preserved: First should come before Second")
	}
}

func TestDedupeShortBlocksKept(t *testing.T) {
	content := `OK

This is a longer paragraph that should be tracked.

OK

This is a longer paragraph that should be tracked.`

	result := Dedupe(content)

	if strings.Count(result.Content, "OK") != 2 {
		t.Errorf("Short blocks should be kept, got content: %s", result.Content)
	}

	if strings.Count(result.Content, "longer paragraph") > 1 {
		t.Error("Long duplicate should be removed")
	}
}

func TestDedupeEmptyContent(t *testing.T) {
	result := Dedupe("")

	if result.Content != "" {
		t.Error("Empty input should produce empty output")
	}
	if result.DuplicatesFound != 0 {
		t.Error("No duplicates in empty content")
	}
}

func TestDedupeNoDuplicates(t *testing.T) {
	content := `# Title

First paragraph with unique content.

# Another Section

Second paragraph with different content.`

	result := Dedupe(content)

	if result.DuplicatesFound != 0 {
		t.Errorf("Expected no duplicates, got %d", result.DuplicatesFound)
	}

	if result.Content != content {
		t.Error("Content should be unchanged when no duplicates")
	}
}

func TestDedupeClassifiesSignals(t *testing.T) {
	content := `Home | About | Contact | Menu

Main article content goes here.

Home | About | Contact | Menu

Footer with copyright notice.

Footer with copyright notice.`

	result := Dedupe(content)

	if result.DuplicatesFound != 2 {
		t.Errorf("Expected 2 duplicates, got %d", result.DuplicatesFound)
	}

	hasNav := false
	hasFooter := false
	for _, signal := range result.DuplicateSignals {
		if signal == "nav" {
			hasNav = true
		}
		if signal == "footer" {
			hasFooter = true
		}
	}

	if !hasNav {
		t.Error("Should detect nav duplicate")
	}
	if !hasFooter {
		t.Error("Should detect footer duplicate")
	}
}

func TestDedupeHeadingDuplicates(t *testing.T) {
	content := `# Related Articles

Some content here that is repeated below.

# Products

Product listing.

# Related Articles

Some content here that is repeated below.`

	result := Dedupe(content)

	if strings.Count(result.Content, "# Related Articles") > 1 {
		t.Error("Duplicate heading should be removed")
	}
	if strings.Count(result.Content, "repeated below") > 1 {
		t.Error("Duplicate content should be removed")
	}

	if !strings.Contains(result.Content, "# Products") {
		t.Error("Products heading should be preserved")
	}
}

func TestDedupeLinesBasic(t *testing.T) {
	content := `Line one
Line two
Line one
Line three
line two`

	result := DedupeLines(content)

	if strings.Count(result, "Line one") > 1 {
		t.Error("Duplicate line should be removed")
	}

	lineCount := 0
	for _, line := range strings.Split(result, "\n") {
		if strings.Contains(strings.ToLower(line), "line two") {
			lineCount++
		}
	}
	if lineCount > 1 {
		t.Error("Case-insensitive duplicate should be removed")
	}
}

func TestNearDuplicateScoreIdentical(t *testing.T) {
	score := NearDuplicateScore("Hello world", "Hello world")
	if score != 100 {
		t.Errorf("Identical strings should score 100, got %d", score)
	}
}

func TestNearDuplicateScorePartial(t *testing.T) {
	score := NearDuplicateScore("The quick brown fox", "The quick red fox")

	if score < 50 {
		t.Errorf("Similar strings should score >50, got %d", score)
	}
	if score == 100 {
		t.Error("Non-identical strings should not score 100")
	}
}

func TestNearDuplicateScoreEmpty(t *testing.T) {
	score := NearDuplicateScore("", "Hello")
	if score != 0 {
		t.Errorf("Empty comparison should score 0, got %d", score)
	}
}

func TestDedupeFormattingVariations(t *testing.T) {
	content := `**Important notice** about something.

Some other content here.

_Important notice_ about something.`

	result := Dedupe(content)

	if result.DuplicatesFound != 1 {
		t.Errorf("Expected 1 duplicate (formatting variation), got %d", result.DuplicatesFound)
	}
}

func TestDedupeWithOptions(t *testing.T) {
	content := `Short

This is content that should be tracked for duplicates.

Short

This is content that should be tracked for duplicates.`

	opts := DedupeOptions{
		MinBlockLen:         3,
		NormalizeWhitespace: true,
		CaseSensitive:       false,
	}

	result := DedupeWithOptions(content, opts)

	if result.DuplicatesFound != 2 {
		t.Errorf("With low MinBlockLen, expected 2 duplicates, got %d", result.DuplicatesFound)
	}
}

func TestDedupePreservesWhitespace(t *testing.T) {
	content := `# Title

Paragraph one.

# Another Title

Paragraph two.`

	result := Dedupe(content)

	if !strings.Contains(result.Content, "\n\n") {
		t.Error("Should preserve paragraph separation")
	}
}

// -----------------------------------------------------------------------------
// simhash / near-duplicate tests
// -----------------------------------------------------------------------------

func TestSimHash_IdenticalBlocksMatch(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog while the morning sun rises gently above the horizon today."
	if len(text) < 80 {
		t.Fatalf("fixture too short: %d", len(text))
	}
	a := simhash(text)
	b := simhash(text)
	d := hammingDistance(a, b)
	if d != 0 {
		t.Fatalf("identical blocks should have distance 0, got %d", d)
	}
	t.Logf("identical distance = %d", d)
}

func TestSimHash_OneWordDifferenceMatches(t *testing.T) {
	// Realistic "templated boilerplate with one word swapped" — the kind of
	// near-dup we actually need to catch. simhash distance for token-shingled
	// text correlates with shingle-set Jaccard distance, so very short blocks
	// (~80 chars / ~14 tokens) produce noisy distances when a mid-sentence
	// word changes (it affects shingleSize shingles out of ~10 — a large
	// fraction). Paragraph-length text is where simhash earns its keep, and
	// matches the real-world target (related-article widgets, repeated CTAs).
	a := "Lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua ut enim ad minim veniam quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur excepteur sint occaecat cupidatat non proident sunt in culpa qui officia deserunt mollit anim id est laborum"
	b := strings.Replace(a, "Lorem", "Lorum", 1)
	if len(a) < 80 || len(b) < 80 {
		t.Fatalf("fixtures too short: a=%d b=%d", len(a), len(b))
	}
	d := hammingDistance(simhash(a), simhash(b))
	if d > nearDupHammingThreshold {
		t.Fatalf("one-word-diff blocks should be within %d bits, got %d", nearDupHammingThreshold, d)
	}
	t.Logf("one-word diff distance = %d (threshold %d, len=%d)", d, nearDupHammingThreshold, len(a))
}

func TestSimHash_DifferentBlocksDifferent(t *testing.T) {
	a := "The quick brown fox jumps over the lazy dog while the morning sun rises gently above the silent horizon today."
	b := "Database connection pooling reduces latency for high throughput services when configured with sensible idle limits."
	d := hammingDistance(simhash(a), simhash(b))
	if d <= 10 {
		t.Fatalf("unrelated blocks should differ by >10 bits, got %d", d)
	}
	t.Logf("unrelated distance = %d", d)
}

func TestDedupe_NearDuplicateCollapsed(t *testing.T) {
	// Templated related-article cards. The stable boilerplate is long enough
	// that a single date/name swap stays within the 3-bit Hamming threshold.
	// This mirrors real-world templates on news/blog index pages.
	body := strings.Repeat("Discover insights about distributed systems and resilient architecture from our engineering team in this featured article from the archive. ", 4)
	content := "# Main Article\n\n" +
		"The main content of the article that we are reading right now today.\n\n" +
		"## Related Articles\n\n" +
		body + "Published March 14 2026.\n\n" +
		body + "Published March 15 2026.\n\n" +
		body + "Published March 16 2026."

	result := Dedupe(content)
	if result.NearDuplicatesFound != 2 {
		t.Fatalf("expected 2 near-duplicates collapsed, got %d (content=%q)", result.NearDuplicatesFound, result.Content)
	}
	// The phrase appears 4 times within each surviving block (boilerplate
	// repetition). With only 1 surviving block, expect exactly 4 occurrences.
	if count := strings.Count(result.Content, "Discover insights about distributed systems"); count != 4 {
		t.Fatalf("expected 4 phrase occurrences (one surviving block × 4 repetitions), got %d", count)
	}
}

func TestDedupe_ShortBlocksNotComparedFuzzy(t *testing.T) {
	// Two ~30-char labels with one-char difference. Below 40-char floor —
	// must remain distinct.
	content := `Subscribe to newsletter A now

Subscribe to newsletter B now`

	opts := DefaultDedupeOptions()
	opts.MinBlockLen = 10 // make sure exact-dedupe would track them
	result := DedupeWithOptions(content, opts)
	if result.NearDuplicatesFound != 0 {
		t.Fatalf("short blocks must not trigger near-dup, got %d", result.NearDuplicatesFound)
	}
	if !strings.Contains(result.Content, "newsletter A") || !strings.Contains(result.Content, "newsletter B") {
		t.Fatalf("both short labels should survive, got %q", result.Content)
	}
}

func TestDedupe_NearDupCountReported(t *testing.T) {
	body := strings.Repeat("Discover insights about distributed systems and resilient architecture from our engineering team in this featured article from the archive. ", 4)
	content := body + "Published March 14 2026.\n\n" + body + "Published March 15 2026."

	result := Dedupe(content)
	if result.NearDuplicatesFound != 1 {
		t.Fatalf("expected NearDuplicatesFound == 1, got %d", result.NearDuplicatesFound)
	}
	if len(result.NearDuplicateSignals) == 0 {
		t.Fatalf("expected NearDuplicateSignals to be populated")
	}
}

func TestDedupe_NearDupDisabled(t *testing.T) {
	body := strings.Repeat("Discover insights about distributed systems and resilient architecture from our engineering team in this featured article from the archive. ", 4)
	content := body + "Published March 14 2026.\n\n" +
		body + "Published March 15 2026.\n\n" +
		body + "Published March 16 2026."

	opts := DefaultDedupeOptions()
	opts.NearDup = false
	result := DedupeWithOptions(content, opts)

	if result.NearDuplicatesFound != 0 {
		t.Fatalf("near-dup disabled should yield 0 near-duplicates, got %d", result.NearDuplicatesFound)
	}
	// 3 surviving blocks × 4 repetitions each = 12 phrase occurrences.
	if count := strings.Count(result.Content, "Discover insights about distributed systems"); count != 12 {
		t.Fatalf("all three blocks should survive when near-dup disabled (3×4=12 occurrences), got %d", count)
	}
}

func TestDedupeRepeatedCards(t *testing.T) {
	content := `# Main Article

The main content of the article.

## Related Articles

Article Title - Brief description of the article with some details about this topic.

Another Title - Different article description here with enough text to be tracked.

Article Title - Brief description of the article with some details about this topic.

Third Article - Yet another article description with sufficient length for tracking.`

	result := Dedupe(content)

	if result.DuplicatesFound == 0 {
		t.Error("Should detect repeated card")
	}
}
