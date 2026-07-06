package engine

import (
	"reflect"
	"testing"
)

func patternsOf(groups []PatternGroup) []string {
	out := make([]string, len(groups))
	for i, g := range groups {
		out[i] = g.Pattern
	}
	return out
}

func findGroup(groups []PatternGroup, pattern string) (PatternGroup, bool) {
	for _, g := range groups {
		if g.Pattern == pattern {
			return g, true
		}
	}
	return PatternGroup{}, false
}

func TestGroupByPatternMixedCorpus(t *testing.T) {
	urls := []string{
		"https://ex.com/",
		"https://ex.com/about",
		"https://ex.com/contact",
		"https://ex.com/blog/2026/my-post",
		"https://ex.com/blog/2025/another-story",
		"https://ex.com/products/12345/detail",
		"https://ex.com/products/67890/detail",
		"https://ex.com/blog?page=1",
		"https://ex.com/blog?page=2",
	}

	groups := groupByPattern(urls)

	wantPatterns := []string{
		"/",
		"/about",
		"/blog",
		"/blog/*/*",
		"/contact",
		"/products/*/detail",
	}
	if got := patternsOf(groups); !reflect.DeepEqual(got, wantPatterns) {
		t.Fatalf("patterns = %v, want %v", got, wantPatterns)
	}

	if g, ok := findGroup(groups, "/blog/*/*"); !ok || g.TotalInSitemap != 2 {
		t.Errorf("/blog/*/* group = %+v, want 2 members", g)
	}
	if g, ok := findGroup(groups, "/products/*/detail"); !ok || g.TotalInSitemap != 2 {
		t.Errorf("/products/*/detail group = %+v, want 2 members", g)
	}
	// Paginated URLs share one pattern but stay distinct members.
	if g, ok := findGroup(groups, "/blog"); !ok || g.TotalInSitemap != 2 {
		t.Errorf("/blog group = %+v, want 2 paginated members", g)
	}
}

func TestGroupByPatternOrderIndependent(t *testing.T) {
	a := []string{
		"https://ex.com/products/1/detail",
		"https://ex.com/products/2/detail",
		"https://ex.com/about",
	}
	b := []string{
		"https://ex.com/about",
		"https://ex.com/products/2/detail",
		"https://ex.com/products/1/detail",
	}
	if !reflect.DeepEqual(groupByPattern(a), groupByPattern(b)) {
		t.Errorf("grouping is order-dependent:\n a=%+v\n b=%+v", groupByPattern(a), groupByPattern(b))
	}
}

func TestGroupByPatternNormalization(t *testing.T) {
	// Trailing slash, fragment, and duplicate collapse to one member.
	urls := []string{
		"https://ex.com/about",
		"https://ex.com/about/",
		"https://ex.com/about#team",
		"https://ex.com/about",
	}
	groups := groupByPattern(urls)
	g, ok := findGroup(groups, "/about")
	if !ok {
		t.Fatalf("missing /about group in %v", patternsOf(groups))
	}
	if g.TotalInSitemap != 1 || len(g.URLs) != 1 {
		t.Errorf("/about members = %v, want 1 after normalization", g.URLs)
	}
}

func TestIsVariableSegment(t *testing.T) {
	cases := map[string]bool{
		"12345":                                true,
		"2026":                                 true,
		"2026-07-04":                           true,
		"my-post":                              true,
		"post123":                              true,
		"550e8400-e29b-41d4-a716-446655440000": true,
		"deadbeefcafe0":                        true,
		"detail":                               false,
		"about":                                false,
		"blog":                                 false,
		"products":                             false,
	}
	for seg, want := range cases {
		if got := isVariableSegment(seg); got != want {
			t.Errorf("isVariableSegment(%q) = %v, want %v", seg, got, want)
		}
	}
}

func TestCollapseSiblingLeavesDocsTree(t *testing.T) {
	urls := []string{
		"https://x.com/content-management/archetypes",
		"https://x.com/content-management/comments",
		"https://x.com/content-management/diagrams",
		"https://x.com/content-management/emojis",
		"https://x.com/hugo-pipes/introduction",
		"https://x.com/about",
	}
	groups := groupByPattern(urls)

	g, ok := findGroup(groups, "/*/*")
	if !ok {
		t.Fatalf("expected collapsed /*/* group, got %v", patternsOf(groups))
	}
	if g.TotalInSitemap != 5 {
		t.Errorf("collapsed group has %d members, want 5: %v", g.TotalInSitemap, g.URLs)
	}
	if _, ok := findGroup(groups, "/about"); !ok {
		t.Errorf("root-level /about must survive collapsing, got %v", patternsOf(groups))
	}
	if len(groups) != 2 {
		t.Errorf("want 2 groups (/*/* and /about), got %v", patternsOf(groups))
	}
}

func TestCollapseSiblingLeavesLiteralParent(t *testing.T) {
	urls := []string{
		"https://x.com/docs/install",
		"https://x.com/docs/config",
		"https://x.com/docs/deploy",
		"https://x.com/docs/faq",
	}
	groups := groupByPattern(urls)
	g, ok := findGroup(groups, "/docs/*")
	if !ok || g.TotalInSitemap != 4 {
		t.Fatalf("want one /docs/* group with 4 members, got %v", patternsOf(groups))
	}
}

func TestCollapseThresholdRespected(t *testing.T) {
	urls := []string{
		"https://x.com/docs/install",
		"https://x.com/docs/config",
		"https://x.com/legal/terms",
	}
	groups := groupByPattern(urls)
	if _, ok := findGroup(groups, "/docs/*"); ok {
		t.Errorf("2 siblings are below the threshold, must not collapse: %v", patternsOf(groups))
	}
	if len(groups) != 3 {
		t.Errorf("want 3 untouched singleton groups, got %v", patternsOf(groups))
	}
}

func TestCollapseTopLevelPagesExempt(t *testing.T) {
	urls := []string{
		"https://x.com/about",
		"https://x.com/pricing",
		"https://x.com/contact",
		"https://x.com/blog",
	}
	groups := groupByPattern(urls)
	if len(groups) != 4 {
		t.Errorf("top-level pages must never merge, got %v", patternsOf(groups))
	}
}

func TestCollapseMergesIntoExistingWildcardGroup(t *testing.T) {
	urls := []string{
		"https://x.com/docs/getting-started", // dashed slug → /docs/* in pass one
		"https://x.com/docs/install",
		"https://x.com/docs/config",
		"https://x.com/docs/deploy",
	}
	groups := groupByPattern(urls)
	g, ok := findGroup(groups, "/docs/*")
	if !ok || g.TotalInSitemap != 4 {
		t.Fatalf("want singletons merged into existing /docs/* (4 members), got %v", patternsOf(groups))
	}
	if len(groups) != 1 {
		t.Errorf("want a single /docs/* group, got %v", patternsOf(groups))
	}
}

func TestCollapseMultiMemberPatternsUntouched(t *testing.T) {
	urls := []string{
		"https://x.com/products/1/detail",
		"https://x.com/products/2/detail",
		"https://x.com/products/1/specs",
		"https://x.com/products/2/specs",
		"https://x.com/products/1/reviews",
		"https://x.com/products/2/reviews",
	}
	groups := groupByPattern(urls)
	for _, want := range []string{"/products/*/detail", "/products/*/specs", "/products/*/reviews"} {
		if _, ok := findGroup(groups, want); !ok {
			t.Errorf("multi-member pattern %s must not be collapsed, got %v", want, patternsOf(groups))
		}
	}
}
