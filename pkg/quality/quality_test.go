package quality

import (
	"math"
	"testing"
)

func TestCountHeadings(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		expected int
	}{
		{"single h1", "# Main Title\n\nSome content", 1},
		{"multiple headings", "# Main\n## Section\n### Subsection\nContent", 3},
		{"no headings", "Just some plain text\nwith multiple lines\nbut no headings", 0},
		{"invalid heading format", "#NoSpace\n# ValidHeading", 1},
		{"many headings", "# H1\n## H2\n### H3\n#### H4\n##### H5\n###### H6", 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countHeadings(tt.markdown); got != tt.expected {
				t.Errorf("countHeadings() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestCountParagraphs(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		expected int
	}{
		{"single paragraph", "This is a paragraph.", 1},
		{"multiple paragraphs", "First paragraph.\n\nSecond paragraph.\n\nThird paragraph.", 3},
		{"mixed content", "Para 1\n\n# Heading\n\nPara 2\n\n- List item", 2},
		{"empty string", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countParagraphs(tt.markdown); got != tt.expected {
				t.Errorf("countParagraphs() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestCountListItems(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		expected int
	}{
		{"unordered list", "- Item 1\n- Item 2\n- Item 3", 3},
		{"ordered list", "1. First\n2. Second\n3. Third", 3},
		{"mixed lists", "- Bullet\n* Asterisk\n1. Ordered", 3},
		{"no lists", "Just text\nNo list items here", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countListItems(tt.markdown); got != tt.expected {
				t.Errorf("countListItems() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestCountCodeBlocks(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		expected int
	}{
		{"single code block", "Text\n```go\ncode here\n```\nMore text", 1},
		{"multiple code blocks", "```\ncode1\n```\nText\n```\ncode2\n```", 2},
		{"no code blocks", "Just plain markdown\nwith no code", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countCodeBlocks(tt.markdown); got != tt.expected {
				t.Errorf("countCodeBlocks() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestCountLinks(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		expected int
	}{
		{"single link", "Check out [this link](https://example.com)", 1},
		{"multiple links", "[Link 1](https://example.com) and [Link 2](https://example.org)", 2},
		{"no links", "Just plain text with no hyperlinks", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countLinks(tt.markdown); got != tt.expected {
				t.Errorf("countLinks() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestCalculateTextDensity(t *testing.T) {
	tests := []struct {
		name    string
		md      string
		minDens float64
		maxDens float64
	}{
		{"high text density", "This is a lot of plain text with many words and only minimal markdown syntax applied to it.", 0.7, 1.0},
		{"moderate density", "# Title\n\nSome text here\n\n- List item\n- Another item", 0.7, 1.0},
		{"empty string", "", 0.0, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateTextDensity(tt.md)
			if got < tt.minDens || got > tt.maxDens {
				t.Errorf("calculateTextDensity() = %f, want between %f and %f", got, tt.minDens, tt.maxDens)
			}
		})
	}
}

func TestCompute_HighQuality(t *testing.T) {
	md := "# Main Article Title\n\nFirst paragraph with substantial content.\n\n## Section 1\n\nIntro text.\n\n- Point 1\n- Point 2\n- Point 3\n\n### Subsection\n\nMore content.\n\n[Link](https://example.com)\n\n## Section 2\n\nAnother section.\n\n" + makeContentParagraphs(10)
	m := Compute(md)
	if m.Score < 70 {
		t.Errorf("High quality content should score > 70, got %f", m.Score)
	}
}

func TestCompute_LowQuality(t *testing.T) {
	m := Compute("Short content")
	if m.Score > 10 {
		t.Errorf("Very short content should score low, got %f", m.Score)
	}
}

func TestComputeScore_Bounds(t *testing.T) {
	tests := []Metrics{
		{},
		{ContentLength: 5000, HeadingCount: 10, ParagraphCount: 50, ListCount: 10, CodeBlockCount: 5, LinkCount: 20, TextDensity: 1.0},
	}
	for _, m := range tests {
		score := computeScore(m)
		if score < 0 || score > 100 {
			t.Errorf("Score out of bounds: %f", score)
		}
	}
}

func TestCompute_EdgeCases(t *testing.T) {
	for _, md := range []string{"", "   \n\n  \t  ", "# Title\n\nContent with [link](https://example.com)"} {
		m := Compute(md)
		if math.IsNaN(m.Score) || math.IsInf(m.Score, 0) || m.Score < 0 || m.Score > 100 {
			t.Errorf("Invalid score %f for %q", m.Score, md)
		}
	}
}

func makeContentParagraphs(count int) string {
	content := ""
	for i := 0; i < count; i++ {
		content += "This is a paragraph with substantial content to reach minimum word count. It contains multiple sentences that would typically appear in a real document. The goal is to have enough text to properly evaluate extraction quality.\n\n"
	}
	return content
}
