package engine

import (
	"fmt"
	"regexp"
	"strings"
)

type LinkRetention int

const (
	LinkRetentionAll LinkRetention = iota
	LinkRetentionNone
	LinkRetentionText
	LinkRetentionFooter
)

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

var linkInlineRE = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]*)\)`)

var doubleSpaceRE = regexp.MustCompile(`  +`)

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

		if start > 0 && masked[start-1] == '!' {
			continue
		}

		b.WriteString(masked[cursor:start])

		switch mode {
		case LinkRetentionNone:
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
		out = doubleSpaceRE.ReplaceAllString(out, " ")
	}

	return unmaskCode(out, codeStore)
}
