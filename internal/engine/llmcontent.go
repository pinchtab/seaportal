package engine

import (
	"regexp"
	"strings"
)

var llmContentPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)instructions?\s+for\s+LLMs?`),
	regexp.MustCompile(`(?i)instructions?\s+for\s+(AI|language\s+model|large\s+language)`),
	regexp.MustCompile(`(?i)LLM\s+instructions?`),
	regexp.MustCompile(`(?i)AI\s+agent\s+instructions?`),
}

func detectLLMContent(content string) bool {
	if len(content) == 0 {
		return false
	}
	lower := strings.ToLower(content)
	if !strings.Contains(lower, "llm") && !strings.Contains(lower, "language model") && !strings.Contains(lower, "ai agent") {
		return false
	}
	for _, re := range llmContentPatterns {
		if re.MatchString(content) {
			return true
		}
	}
	return false
}
