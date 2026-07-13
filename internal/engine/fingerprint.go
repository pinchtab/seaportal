package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

func SemanticFingerprint(content string) string {
	normalized := normalizeForFingerprint(content)
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:16])
}

func ContentChanged(oldContent, newContent string) bool {
	return SemanticFingerprint(oldContent) != SemanticFingerprint(newContent)
}

func ChangeSignificance(oldContent, newContent string) int {
	oldNorm := normalizeForFingerprint(oldContent)
	newNorm := normalizeForFingerprint(newContent)

	if oldNorm == newNorm {
		return 0
	}

	oldWords := extractWords(oldNorm)
	newWords := extractWords(newNorm)

	if len(oldWords) == 0 && len(newWords) == 0 {
		return 0
	}

	oldSet := make(map[string]bool)
	for _, w := range oldWords {
		oldSet[w] = true
	}

	newSet := make(map[string]bool)
	for _, w := range newWords {
		newSet[w] = true
	}

	intersection := 0
	for w := range oldSet {
		if newSet[w] {
			intersection++
		}
	}

	union := len(oldSet) + len(newSet) - intersection
	if union == 0 {
		return 0
	}

	similarity := float64(intersection) / float64(union)
	significance := int((1 - similarity) * 100)

	oldLines := len(strings.Split(oldNorm, "\n"))
	newLines := len(strings.Split(newNorm, "\n"))
	lineDiff := abs(oldLines - newLines)
	if lineDiff > 10 {
		significance = min(100, significance+20)
	} else if lineDiff > 5 {
		significance = min(100, significance+10)
	}

	return significance
}

func normalizeForFingerprint(content string) string {
	s := content
	s = idPatterns.ReplaceAllString(s, " ")
	s = timestampPatterns.ReplaceAllString(s, " ")
	s = counterPatterns.ReplaceAllString(s, " ")
	s = relativeTimePatterns.ReplaceAllString(s, " ")
	s = versionPatterns.ReplaceAllString(s, " ")
	s = wsRunRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(strings.ToLower(s))
}

func extractWords(s string) []string {
	words := strings.Fields(s)
	var result []string
	for _, w := range words {
		if len(w) >= 3 {
			result = append(result, w)
		}
	}
	sort.Strings(result)
	return result
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

var (
	timestampPatterns = regexp.MustCompile(`(?i)` +
		`\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2}(:\d{2})?(\.\d+)?(Z|[+-]\d{2}:?\d{2})?)?` +
		`|\d{1,2}/\d{1,2}/\d{2,4}` +
		`|\d{1,2}\s+(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\s+\d{2,4}` +
		`|(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\s+\d{1,2},?\s+\d{2,4}` +
		`|\d{1,2}:\d{2}(:\d{2})?\s*(am|pm)?` +
		`|(today|yesterday|tomorrow)`,
	)

	counterPatterns = regexp.MustCompile(`\b\d{3,}\b`)

	idPatterns = regexp.MustCompile(`(?i)` +
		`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}` +
		`|[0-9a-f]{32,}`,
	)

	relativeTimePatterns = regexp.MustCompile(`(?i)` +
		`\d+\s*(second|minute|hour|day|week|month|year)s?\s+ago` +
		`|just\s+now` +
		`|moments?\s+ago` +
		`|a\s+(few|couple)\s+(seconds?|minutes?|hours?|days?)\s+ago`,
	)

	versionPatterns = regexp.MustCompile(`\bv?\d+\.\d+(\.\d+)*\b`)
)
