package engine

import (
	"hash/fnv"
	"math/rand"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// sample selects which discovered URLs actually get fetched, honoring MaxPages
// (total budget), MaxPerPattern (per-group cap), Full (disable sampling), and
// Include/ExcludePatterns. groups come from groupByPattern (ALP-003).
//
// Determinism: balanced and priority derive purely from the sorted URL set;
// random shuffles with a seed derived from a FNV-1a hash of opts.BaseURL (plus
// the group index), so a given site samples identically across reruns.
func sample(groups []PatternGroup, opts ScrapeOptions) []string {
	o := opts.normalized()
	filtered := filterGroups(groups, o.IncludePatterns, o.ExcludePatterns)

	if o.Full {
		// Full disables per-pattern sampling but MaxPages stays a real upper
		// bound: sitemap discovery can return thousands of URLs, and without
		// this cap --full would fetch them all (the crawl-fallback path already
		// caps discovery at MaxPages, so this keeps both paths consistent).
		urls := allURLs(filtered)
		if len(urls) > o.MaxPages {
			urls = urls[:o.MaxPages]
		}
		return urls
	}

	switch o.SampleStrategy {
	case SampleRandom:
		seed := seedFor(o.BaseURL)
		return roundRobin(filtered, o.MaxPages, o.MaxPerPattern, func(i int, g PatternGroup) []string {
			return seededShuffle(g.URLs, seed+int64(i))
		})
	case SamplePriority:
		return samplePriority(filtered, o.MaxPages, o.MaxPerPattern)
	default: // balanced
		return roundRobin(filtered, o.MaxPages, o.MaxPerPattern, func(_ int, g PatternGroup) []string {
			return g.URLs // already sorted, deterministic
		})
	}
}

// roundRobin spreads the budget across groups one URL at a time so no single
// large group starves the others. order yields each group's URLs in the desired
// order (sorted for balanced, shuffled for random), truncated to maxPerPattern.
func roundRobin(groups []PatternGroup, maxPages, maxPerPattern int, order func(i int, g PatternGroup) []string) []string {
	lists := make([][]string, len(groups))
	for i, g := range groups {
		urls := order(i, g)
		if maxPerPattern > 0 && len(urls) > maxPerPattern {
			urls = urls[:maxPerPattern]
		}
		lists[i] = urls
	}

	var out []string
	for depth := 0; len(out) < maxPages; depth++ {
		progressed := false
		for _, list := range lists {
			if depth >= len(list) {
				continue
			}
			out = append(out, list[depth])
			progressed = true
			if len(out) >= maxPages {
				return out
			}
		}
		if !progressed {
			break
		}
	}
	return out
}

// samplePriority guarantees the homepage and one representative per top-level
// section first (budget permitting), then fills the rest shallowest-first.
func samplePriority(groups []PatternGroup, maxPages, maxPerPattern int) []string {
	type item struct {
		url, pattern, section string
		depth                 int
	}
	var items []item
	for _, g := range groups {
		for _, u := range g.URLs {
			items = append(items, item{u, g.Pattern, sectionOf(u), depthOf(u)})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].depth != items[j].depth {
			return items[i].depth < items[j].depth
		}
		return items[i].url < items[j].url
	})

	perGroup := map[string]int{}
	picked := map[string]bool{}
	var out []string
	canTake := func(it item) bool {
		return !picked[it.url] && len(out) < maxPages &&
			(maxPerPattern <= 0 || perGroup[it.pattern] < maxPerPattern)
	}
	take := func(it item) {
		picked[it.url] = true
		perGroup[it.pattern]++
		out = append(out, it.url)
	}

	// Pass 1: homepage(s).
	for _, it := range items {
		if it.depth == 0 && canTake(it) {
			take(it)
		}
	}
	// Pass 2: one representative per top-level section.
	seenSection := map[string]bool{}
	for _, it := range items {
		if len(out) >= maxPages {
			break
		}
		if it.section == "" || seenSection[it.section] {
			continue
		}
		if canTake(it) {
			take(it)
			seenSection[it.section] = true
		}
	}
	// Pass 3: fill remaining budget shallowest-first.
	for _, it := range items {
		if len(out) >= maxPages {
			break
		}
		if canTake(it) {
			take(it)
		}
	}
	return out
}

// filterGroups drops URLs failing the include/exclude globs (matched against the
// URL path). A URL is kept when it matches some include (or none are given) and
// matches no exclude. Empty groups are removed.
func filterGroups(groups []PatternGroup, include, exclude []string) []PatternGroup {
	inc := compileGlobs(include)
	exc := compileGlobs(exclude)
	var out []PatternGroup
	for _, g := range groups {
		var kept []string
		for _, u := range g.URLs {
			p := pathOf(u)
			if len(inc) > 0 && !matchesAny(inc, p) {
				continue
			}
			if matchesAny(exc, p) {
				continue
			}
			kept = append(kept, u)
		}
		if len(kept) > 0 {
			out = append(out, PatternGroup{Pattern: g.Pattern, URLs: kept, TotalInSitemap: g.TotalInSitemap})
		}
	}
	return out
}

// allURLs returns every URL across groups, de-duplicated and sorted (Full mode).
func allURLs(groups []PatternGroup) []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range groups {
		for _, u := range g.URLs {
			if !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
		}
	}
	sort.Strings(out)
	return out
}

func seededShuffle(urls []string, seed int64) []string {
	cp := append([]string(nil), urls...)
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
	return cp
}

// seedFor derives a stable per-site seed from the base URL (FNV-1a), so random
// sampling is reproducible for a given site without depending on wall-clock.
func seedFor(base string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(base))
	return int64(h.Sum64())
}

func pathOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Path == "" {
		return "/"
	}
	return u.Path
}

func depthOf(raw string) int {
	p := strings.Trim(pathOf(raw), "/")
	if p == "" {
		return 0
	}
	return strings.Count(p, "/") + 1
}

func sectionOf(raw string) string {
	p := strings.Trim(pathOf(raw), "/")
	if p == "" {
		return ""
	}
	return strings.SplitN(p, "/", 2)[0]
}

// compileGlobs splits comma-separated glob strings and compiles each to a regex.
// `*` matches within a path segment, `**` matches across segments, `?` matches a
// single non-slash char.
func compileGlobs(patterns []string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, raw := range patterns {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if re := globToRegex(part); re != nil {
				out = append(out, re)
			}
		}
	}
	return out
}

func matchesAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func globToRegex(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch glob[i] {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil
	}
	return re
}
