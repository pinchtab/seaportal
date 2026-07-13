package engine

import (
	"fmt"
	"regexp"
	"strings"
)

func ConvertLinksToCitations(md string) string {
	if md == "" {
		return md
	}

	masked, codeStore := maskCode(md)

	linkRE := regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)

	var (
		urls    []string
		indexes = make(map[string]int)
	)

	var b strings.Builder
	b.Grow(len(masked) + 128)
	matches := linkRE.FindAllStringSubmatchIndex(masked, -1)
	cursor := 0
	converted := 0

	for _, m := range matches {
		start, end := m[0], m[1]
		textStart, textEnd := m[2], m[3]
		urlStart, urlEnd := m[4], m[5]

		if start > 0 && masked[start-1] == '!' {
			continue
		}

		b.WriteString(masked[cursor:start])

		text := masked[textStart:textEnd]
		urlStr := masked[urlStart:urlEnd]

		n, seen := indexes[urlStr]
		if !seen {
			urls = append(urls, urlStr)
			n = len(urls)
			indexes[urlStr] = n
		}

		fmt.Fprintf(&b, "%s ⟨%d⟩", text, n)
		cursor = end
		converted++
	}
	b.WriteString(masked[cursor:])

	if converted == 0 {
		return md
	}

	result := unmaskCode(b.String(), codeStore)

	var refs strings.Builder
	refs.WriteString("\n\n## References\n\n")
	for i, u := range urls {
		fmt.Fprintf(&refs, "%d. <%s>\n", i+1, u)
	}

	return result + refs.String()
}

var (
	fencedCodeRE = regexp.MustCompile("(?s)```[^\\n]*\\n.*?```")
	inlineCodeRE = regexp.MustCompile("`[^`\\n]+`")
)

func maskCode(md string) (string, []string) {
	var store []string

	replace := func(re *regexp.Regexp, s string) string {
		return re.ReplaceAllStringFunc(s, func(match string) string {
			token := fmt.Sprintf("\x00CB%d\x00", len(store))
			store = append(store, match)
			return token
		})
	}

	masked := replace(fencedCodeRE, md)
	masked = replace(inlineCodeRE, masked)
	return masked, store
}

func unmaskCode(masked string, store []string) string {
	if len(store) == 0 {
		return masked
	}
	for i, original := range store {
		token := fmt.Sprintf("\x00CB%d\x00", i)
		masked = strings.ReplaceAll(masked, token, original)
	}
	return masked
}
