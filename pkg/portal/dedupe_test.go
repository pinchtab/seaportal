package portal

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

	// 3 duplicates: "This is the main content", "# Navigation", "Home | About | Contact"
	if result.DuplicatesFound != 3 {
		t.Errorf("Expected 3 duplicates, got %d", result.DuplicatesFound)
	}

	// Check that duplicates are removed
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

	// First occurrence should be preserved
	if !strings.Contains(result.Content, "# First") {
		t.Error("First heading should be preserved")
	}
	if !strings.Contains(result.Content, "First paragraph") {
		t.Error("First paragraph should be preserved")
	}

	// Check structure: First should come before Second
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

	// Short "OK" blocks should be kept (under MinBlockLen)
	if strings.Count(result.Content, "OK") != 2 {
		t.Errorf("Short blocks should be kept, got content: %s", result.Content)
	}

	// Long duplicate should be removed
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

	// Should classify nav and footer duplicates
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
	// Same heading with same content underneath = duplicate
	content := `# Related Articles

Some content here that is repeated below.

# Products

Product listing.

# Related Articles

Some content here that is repeated below.`

	result := Dedupe(content)

	// The heading itself should be deduplicated (along with its matching content)
	if strings.Count(result.Content, "# Related Articles") > 1 {
		t.Error("Duplicate heading should be removed")
	}
	if strings.Count(result.Content, "repeated below") > 1 {
		t.Error("Duplicate content should be removed")
	}

	// Products section should be preserved
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

	// Case-insensitive: "line two" and "Line two" should dedup
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

	// Should be high similarity (3 of 4 words match)
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
	// Same content with different markdown formatting
	content := `**Important notice** about something.

Some other content here.

_Important notice_ about something.`

	result := Dedupe(content)

	// Should detect these as duplicates despite formatting differences
	if result.DuplicatesFound != 1 {
		t.Errorf("Expected 1 duplicate (formatting variation), got %d", result.DuplicatesFound)
	}
}

func TestDedupeWithOptions(t *testing.T) {
	content := `Short

This is content that should be tracked for duplicates.

Short

This is content that should be tracked for duplicates.`

	// With very low MinBlockLen, "Short" should also be deduped
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

	// Should maintain paragraph separation
	if !strings.Contains(result.Content, "\n\n") {
		t.Error("Should preserve paragraph separation")
	}
}

func TestDedupeRepeatedCards(t *testing.T) {
	// Common pattern: related articles with same template - each as separate block
	content := `# Main Article

The main content of the article.

## Related Articles

Article Title - Brief description of the article with some details about this topic.

Another Title - Different article description here with enough text to be tracked.

Article Title - Brief description of the article with some details about this topic.

Third Article - Yet another article description with sufficient length for tracking.`

	result := Dedupe(content)

	// Should detect the repeated card
	if result.DuplicatesFound == 0 {
		t.Error("Should detect repeated card")
	}
}
