package engine

import (
	"fmt"
	"regexp"
	"strings"
)

// ConvertLinksToCitations rewrites every inline Markdown link `[text](url)`
// into `text ⟨N⟩` and appends a `## References` section listing each unique
// URL once (in document order, 1-based). Identical URLs reuse the same
// number. Fenced code blocks and inline code spans are preserved verbatim,
// so any `[text](url)` inside code is left alone. Image syntax
// `![alt](url)` is also untouched — the leading `!` distinguishes it.
//
// Out of scope for V1:
//   - Reference-style links (`[text][ref]` + `[ref]: url`).
//   - Autolinks (`<https://example.com>`).
//
// If the input contains no convertible inline links, the input is returned
// unchanged (no empty `## References` section is appended).
func ConvertLinksToCitations(md string) string {
	if md == "" {
		return md
	}

	// Mask code regions so links inside them are immune to rewriting.
	// Fenced blocks first (multi-line), then inline code spans.
	masked, codeStore := maskCode(md)

	linkRE := regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)

	var (
		urls    []string
		indexes = make(map[string]int)
	)

	// Index-based walk so we can peek at the preceding byte to skip image
	// syntax (`![alt](url)`), which RE2 can't express via lookbehind.
	var b strings.Builder
	b.Grow(len(masked) + 128)
	matches := linkRE.FindAllStringSubmatchIndex(masked, -1)
	cursor := 0
	converted := 0

	for _, m := range matches {
		start, end := m[0], m[1]
		textStart, textEnd := m[2], m[3]
		urlStart, urlEnd := m[4], m[5]

		// Skip image syntax: ![alt](url) — the `[` is preceded by `!`.
		if start > 0 && masked[start-1] == '!' {
			continue
		}

		// Emit everything up to this match unchanged.
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
		// Nothing converted — return the original, untouched input
		// (don't even round-trip through unmasking; same bytes either way).
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

// maskCode replaces fenced code blocks and inline code spans with sentinel
// tokens of the form "\x00CB<N>\x00" so subsequent text-level rewrites
// can't touch them. Returns the masked string and the originals indexed
// by N (placeholder order).
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

// unmaskCode reverses maskCode by substituting each "\x00CB<N>\x00" token
// back to its original content.
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
