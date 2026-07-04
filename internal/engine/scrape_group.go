package engine

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// PatternGroup is a cluster of discovered URLs that share a path template, so a
// horizontal site (thousands of /blog/* pages) collapses into a handful of
// representative groups the sampler (ALP-004) can draw from.
type PatternGroup struct {
	// Pattern is the path template, variable segments replaced by "*"
	// (e.g. "/products/*/detail").
	Pattern string
	// URLs are the de-duplicated member URLs, sorted for determinism.
	URLs []string
	// TotalInSitemap is how many distinct URLs fell into this group.
	TotalInSitemap int
}

var (
	reAllDigits = regexp.MustCompile(`^\d+$`)
	reUUID      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	reLongHex   = regexp.MustCompile(`^[0-9a-fA-F]{12,}$`)
	reHasDigit  = regexp.MustCompile(`\d`)
	reHasAlpha  = regexp.MustCompile(`[a-zA-Z]`)
)

// isVariableSegment reports whether a path segment looks like a variable id,
// date, uuid, or slug (→ wildcard) rather than a stable structural segment
// like "detail" or "about" (→ kept verbatim).
func isVariableSegment(seg string) bool {
	if seg == "" {
		return false
	}
	switch {
	case reAllDigits.MatchString(seg): // numeric id, year, or date part
		return true
	case reUUID.MatchString(seg):
		return true
	case reLongHex.MatchString(seg): // long hex hash
		return true
	case strings.ContainsAny(seg, "-_"): // multi-token slug (my-post, 2026-07-04)
		return true
	case len(seg) > 24: // very long single token
		return true
	case reHasDigit.MatchString(seg) && reHasAlpha.MatchString(seg): // mixed alnum id (post123)
		return true
	default:
		return false
	}
}

// patternForPath builds the template for a cleaned path.
func patternForPath(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "/"
	}
	segs := strings.Split(trimmed, "/")
	for i, s := range segs {
		if isVariableSegment(s) {
			segs[i] = "*"
		}
	}
	return "/" + strings.Join(segs, "/")
}

// normalizeMember strips the fragment and normalizes the trailing slash so
// cosmetic variants don't create spurious members, while preserving the query
// (distinct ?page=N pages remain distinct). Returns the canonical member URL
// and its path for templating.
func normalizeMember(raw string) (member, path string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", false
	}
	u.Fragment = ""
	u.RawFragment = ""
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	return u.String(), u.Path, true
}

// groupByPattern clusters urls into PatternGroups keyed by their path template.
// It is deterministic and order-independent: the same input set yields the same
// groups (sorted by pattern, members sorted) regardless of input ordering.
func groupByPattern(urls []string) []PatternGroup {
	type acc struct {
		urls map[string]struct{}
		list []string
	}
	byPattern := map[string]*acc{}
	for _, raw := range urls {
		member, path, ok := normalizeMember(raw)
		if !ok || member == "" {
			continue
		}
		pattern := patternForPath(path)
		a := byPattern[pattern]
		if a == nil {
			a = &acc{urls: map[string]struct{}{}}
			byPattern[pattern] = a
		}
		if _, dup := a.urls[member]; dup {
			continue
		}
		a.urls[member] = struct{}{}
		a.list = append(a.list, member)
	}

	out := make([]PatternGroup, 0, len(byPattern))
	for pattern, a := range byPattern {
		sort.Strings(a.list)
		out = append(out, PatternGroup{
			Pattern:        pattern,
			URLs:           a.list,
			TotalInSitemap: len(a.list),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pattern < out[j].Pattern })
	return out
}
