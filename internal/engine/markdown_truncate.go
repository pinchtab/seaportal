package engine

import "strings"

const charsPerToken = 4

func TruncateMarkdownAtParagraph(md string, maxTokens int) (string, bool) {
	if maxTokens <= 0 || md == "" {
		return md, false
	}
	budget := maxTokens * charsPerToken
	if len(md) <= budget {
		return md, false
	}
	cut := strings.LastIndex(md[:budget], "\n\n")
	if cut < 0 {
		cut = strings.LastIndex(md[:budget], "\n")
	}
	if cut < 0 {
		cut = budget
	}
	return md[:cut] + "\n\n*[truncated]*\n", true
}
