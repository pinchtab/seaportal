package engine

import (
	"fmt"
	"regexp"
	"strings"
)

// LinkRetention controls how inline Markdown links `[text](url)` are rendered
// after extraction. The default (LinkRetentionAll) leaves links untouched.
type LinkRetention int

const (
	// LinkRetentionAll keeps inline `[text](url)` as-is (default).
	LinkRetentionAll LinkRetention = iota
	// LinkRetentionNone strips both link text and URL.
	LinkRetentionNone
	// LinkRetentionText keeps the link text, drops the URL.
	LinkRetentionText
	// LinkRetentionFooter delegates to ConvertLinksToCitations:
	// inline links become `text ⟨N⟩` plus a numbered `## References` section.
	LinkRetentionFooter
)

// String returns the canonical lowercase name of the mode.
func (m LinkRetention) String() string {
	switch m {
	case LinkRetentionAll:
		return "all"
	case LinkRetentionNone:
		return "none"
	case LinkRetentionText:
		return "text"
	case LinkRetentionFooter:
		return "footer"
	default:
		return fmt.Sprintf("unknown(%d)", int(m))
	}
}

// ParseLinkRetention parses a mode name. Accepts lowercase
// "none", "text", "all", "footer". Returns an error otherwise.
func ParseLinkRetention(s string) (LinkRetention, error) {
	switch s {
	case "all":
		return LinkRetentionAll, nil
	case "none":
		return LinkRetentionNone, nil
	case "text":
		return LinkRetentionText, nil
	case "footer":
		return LinkRetentionFooter, nil
	default:
		return LinkRetentionAll, fmt.Errorf("invalid link retention mode %q: want one of none|text|all|footer", s)
	}
}

// linkInlineRE matches an inline Markdown link `[text](url)`.
// Mirrors the pattern used by ConvertLinksToCitations.
var linkInlineRE = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]*)\)`)

// doubleSpaceRE collapses runs of 2+ spaces (within a line, not newlines).
var doubleSpaceRE = regexp.MustCompile(`  +`)

// applyLinkRetention rewrites `md` according to `mode`.
//
//   - LinkRetentionAll: returns input unchanged.
//   - LinkRetentionFooter: delegates to ConvertLinksToCitations.
//   - LinkRetentionNone: replaces each `[text](url)` with "" (collapses runs
//     of leftover spaces to a single space).
//   - LinkRetentionText: replaces each `[text](url)` with the bare text.
//
// In every non-Footer mode, image syntax `![alt](url)` is left alone (the
// leading `!` is detected by peeking at the byte before the match), and
// fenced code blocks / inline code spans are preserved verbatim via
// maskCode/unmaskCode from citations.go.
func applyLinkRetention(md string, mode LinkRetention) string {
	if md == "" || mode == LinkRetentionAll {
		return md
	}
	if mode == LinkRetentionFooter {
		return ConvertLinksToCitations(md)
	}

	masked, codeStore := maskCode(md)

	var b strings.Builder
	b.Grow(len(masked))
	matches := linkInlineRE.FindAllStringSubmatchIndex(masked, -1)
	cursor := 0
	changed := false

	for _, m := range matches {
		start, end := m[0], m[1]
		textStart, textEnd := m[2], m[3]

		// Skip image syntax `![alt](url)`.
		if start > 0 && masked[start-1] == '!' {
			continue
		}

		b.WriteString(masked[cursor:start])

		switch mode {
		case LinkRetentionNone:
			// replace with nothing
		case LinkRetentionText:
			b.WriteString(masked[textStart:textEnd])
		}

		cursor = end
		changed = true
	}
	b.WriteString(masked[cursor:])

	if !changed {
		return md
	}

	out := b.String()

	if mode == LinkRetentionNone {
		// Collapse runs of double spaces left where links were removed mid-line.
		// Only collapse spaces (not tabs/newlines) to avoid touching code/poetry.
		out = doubleSpaceRE.ReplaceAllString(out, " ")
	}

	return unmaskCode(out, codeStore)
}
