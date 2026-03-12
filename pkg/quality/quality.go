// Package quality provides extraction quality analysis for markdown content.
package quality

import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

// Metrics holds detailed quality assessment of extracted content
type Metrics struct {
	ContentLength  int     `json:"contentLength"`
	HeadingCount   int     `json:"headingCount"`
	ParagraphCount int     `json:"paragraphCount"`
	ListCount      int     `json:"listCount"`
	CodeBlockCount int     `json:"codeBlockCount"`
	LinkCount      int     `json:"linkCount"`
	TextDensity    float64 `json:"textDensity"`
	Score          float64 `json:"score"`
}

// Compute analyzes markdown content and returns quality metrics
func Compute(markdown string) Metrics {
	m := Metrics{}
	m.ContentLength = len(markdown)
	m.HeadingCount = countHeadings(markdown)
	m.ParagraphCount = countParagraphs(markdown)
	m.ListCount = countListItems(markdown)
	m.CodeBlockCount = countCodeBlocks(markdown)
	m.LinkCount = countLinks(markdown)
	m.TextDensity = calculateTextDensity(markdown)
	m.Score = computeScore(m)
	return m
}

func countHeadings(markdown string) int {
	count := 0
	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") && len(trimmed) > 1 {
			if len(trimmed) > 1 && trimmed[1] == ' ' {
				count++
			} else if len(trimmed) > 2 && trimmed[1] == '#' {
				for i := 0; i < len(trimmed); i++ {
					if trimmed[i] != '#' {
						if trimmed[i] == ' ' {
							count++
						}
						break
					}
				}
			}
		}
	}
	return count
}

func countParagraphs(markdown string) int {
	paragraphs := strings.Split(markdown, "\n\n")
	count := 0
	for _, p := range paragraphs {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") &&
			!strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "*") &&
			!strings.HasPrefix(trimmed, "```") && !strings.HasPrefix(trimmed, ">") {
			count++
		}
	}
	return count
}

func countListItems(markdown string) int {
	count := 0
	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")) ||
			(len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' && trimmed[1] == '.' && trimmed[2] == ' ') {
			count++
		}
	}
	return count
}

func countCodeBlocks(markdown string) int {
	return strings.Count(markdown, "```") / 2
}

func countLinks(markdown string) int {
	linkPattern := regexp.MustCompile(`\[[^\]]+\]\([^\)]+\)`)
	return len(linkPattern.FindAllString(markdown, -1))
}

func calculateTextDensity(markdown string) float64 {
	if len(markdown) == 0 {
		return 0
	}
	wordCount := 0
	inWord := false
	for _, r := range markdown {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if !inWord {
				wordCount++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	textChars := 0
	for _, r := range markdown {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			textChars++
		}
	}
	if wordCount == 0 {
		return 0
	}
	density := float64(textChars) / float64(len(markdown))
	if density > 1.0 {
		density = 1.0
	}
	return density
}

func computeScore(m Metrics) float64 {
	score := 0.0
	if m.ContentLength < 100 {
		return 5.0
	}

	// Content length (0-30 pts)
	if m.ContentLength >= 1000 && m.ContentLength <= 5000 { //nolint:gocritic
		score += 30.0
	} else if m.ContentLength < 1000 {
		score += math.Min(30.0, (float64(m.ContentLength)/1000.0)*30.0)
	} else {
		score += 30.0 * math.Min(1.0, 1.0-(float64(m.ContentLength-5000)/10000.0))
	}

	// Structure (0-40 pts)
	structureScore := 0.0
	if m.HeadingCount > 0 {
		structureScore += 10.0
	}
	if m.ParagraphCount > 3 {
		structureScore += 10.0
	}
	if m.ListCount > 0 {
		structureScore += 10.0
	}
	if m.CodeBlockCount > 0 {
		structureScore += 10.0
	}
	score += math.Min(40.0, structureScore)

	// Links (0-10 pts)
	score += math.Min(10.0, float64(m.LinkCount)*2.0)

	// Text density (0-20 pts)
	if m.TextDensity > 0.6 {
		score += 20.0
	} else {
		score += m.TextDensity * 40.0
	}

	if score > 100.0 {
		score = 100.0
	}
	if score < 0 {
		score = 0
	}
	return score
}
