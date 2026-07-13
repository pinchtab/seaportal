package engine

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

type PatternGroup struct {
	Pattern        string
	URLs           []string
	TotalInSitemap int
}

var (
	reAllDigits = regexp.MustCompile(`^\d+$`)
	reUUID      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	reLongHex   = regexp.MustCompile(`^[0-9a-fA-F]{12,}$`)
	reHasDigit  = regexp.MustCompile(`\d`)
	reHasAlpha  = regexp.MustCompile(`[a-zA-Z]`)
)

func isVariableSegment(seg string) bool {
	if seg == "" {
		return false
	}
	switch {
	case reAllDigits.MatchString(seg):
		return true
	case reUUID.MatchString(seg):
		return true
	case reLongHex.MatchString(seg):
		return true
	case strings.ContainsAny(seg, "-_"):
		return true
	case len(seg) > 24:
		return true
	case reHasDigit.MatchString(seg) && reHasAlpha.MatchString(seg):
		return true
	default:
		return false
	}
}

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
	return collapseSiblingLeaves(out)
}

const minSiblingCollapse = 3

func collapseSiblingLeaves(groups []PatternGroup) []PatternGroup {
	siblings := map[string][]string{}
	for _, g := range groups {
		if g.TotalInSitemap != 1 {
			continue
		}
		idx := strings.LastIndex(g.Pattern, "/")
		if idx <= 0 {
			continue
		}
		if leaf := g.Pattern[idx+1:]; leaf == "" || leaf == "*" {
			continue
		}
		parent := g.Pattern[:idx]
		siblings[parent] = append(siblings[parent], g.Pattern)
	}

	collapse := map[string]string{}
	for parent, pats := range siblings {
		if len(pats) < minSiblingCollapse {
			continue
		}
		for _, p := range pats {
			collapse[p] = parent + "/*"
		}
	}
	if len(collapse) == 0 {
		return groups
	}

	merged := map[string]*PatternGroup{}
	for _, g := range groups {
		target := g.Pattern
		if t, ok := collapse[g.Pattern]; ok {
			target = t
		}
		if m := merged[target]; m != nil {
			m.URLs = append(m.URLs, g.URLs...)
			m.TotalInSitemap += g.TotalInSitemap
			continue
		}
		merged[target] = &PatternGroup{
			Pattern:        target,
			URLs:           append([]string(nil), g.URLs...),
			TotalInSitemap: g.TotalInSitemap,
		}
	}

	out := make([]PatternGroup, 0, len(merged))
	for _, m := range merged {
		sort.Strings(m.URLs)
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pattern < out[j].Pattern })
	return out
}
