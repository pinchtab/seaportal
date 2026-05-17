package engine

import (
	"html"
	"regexp"
	"strings"
)

var citationAuthorMetaRE = regexp.MustCompile(
	`(?is)<meta\b[^>]*?\bname\s*=\s*["'](?:citation_author|dc\.creator)["'][^>]*?>`,
)

var metaContentAttrRE = regexp.MustCompile(`(?is)\bcontent\s*=\s*(?:"([^"]*)"|'([^']*)')`)

var dateLikeBylineRE = regexp.MustCompile(
	`(?i)^\s*\[?(?:submitted\b|v\d+\]?\s+\[?submitted\b|\d{1,2}\s+\w+\s+\d{4}|\d{4}-\d{2}-\d{2}|\w+\s+\d{1,2},\s*\d{4})`,
)

func ExtractMetaAuthors(rawHTML string) []string {
	if rawHTML == "" {
		return nil
	}
	matches := citationAuthorMetaRE.FindAllString(rawHTML, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, tag := range matches {
		c := metaContentAttrRE.FindStringSubmatch(tag)
		if c == nil {
			continue
		}
		val := c[1]
		if val == "" {
			val = c[2]
		}
		val = strings.TrimSpace(html.UnescapeString(val))
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out
}

// applyMetaAuthors was removed; the unified applyMetadata pass in metadata.go
// now owns the **Authors:** prepend logic. ExtractMetaAuthors is retained
// (it's still covered by extract_arxiv_test.go and is a focused author-only
// helper that may be useful to external callers).
