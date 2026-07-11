package engine

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// bigCorpus returns pattern groups for a synthetic site: 1 homepage, 20
// /blog/*/post, 15 /products/*/detail, and 2 flat top-level pages.
func bigCorpus() []PatternGroup {
	urls := []string{"https://ex.com/", "https://ex.com/about", "https://ex.com/contact"}
	for i := 1; i <= 20; i++ {
		urls = append(urls, fmt.Sprintf("https://ex.com/blog/%d/post", i))
	}
	for i := 1; i <= 15; i++ {
		urls = append(urls, fmt.Sprintf("https://ex.com/products/%d/detail", i))
	}
	return groupByPattern(urls)
}

func patternCounts(urls []string) map[string]int {
	m := map[string]int{}
	for _, u := range urls {
		m[patternForPath(pathOf(u))]++
	}
	return m
}

func TestSampleCapsEnforced(t *testing.T) {
	groups := bigCorpus()
	for _, strat := range []SampleStrategy{SampleBalanced, SampleRandom, SamplePriority} {
		opts := ScrapeOptions{BaseURL: "https://ex.com", MaxPages: 10, MaxPerPattern: 3, SampleStrategy: strat}
		got := sample(groups, opts)
		if len(got) > 10 {
			t.Errorf("[%s] total = %d, want <= MaxPages 10", strat, len(got))
		}
		for pat, n := range patternCounts(got) {
			if n > 3 {
				t.Errorf("[%s] pattern %s has %d, want <= MaxPerPattern 3", strat, pat, n)
			}
		}
		if hasDup(got) {
			t.Errorf("[%s] result has duplicates: %v", strat, got)
		}
	}
}

func TestSampleFullBypassesPerPatternCap(t *testing.T) {
	// Full disables the per-pattern cap: with a budget large enough for the
	// whole corpus, every URL comes back despite MaxPerPattern 1.
	groups := bigCorpus()
	opts := ScrapeOptions{BaseURL: "https://ex.com", MaxPages: 100, MaxPerPattern: 1, Full: true}
	got := sample(groups, opts)
	if len(got) != 38 { // 1 + 20 + 15 + 2
		t.Errorf("Full sample = %d URLs, want all 38", len(got))
	}
}

func TestSampleFullRespectsMaxPages(t *testing.T) {
	// Full still honors MaxPages as a total upper bound — a large sitemap must
	// not blow past the budget (ALP-034). This mirrors the crawl-fallback path,
	// which already caps discovery at MaxPages.
	groups := bigCorpus() // 38 URLs
	opts := ScrapeOptions{BaseURL: "https://ex.com", MaxPages: 5, MaxPerPattern: 1, Full: true}
	got := sample(groups, opts)
	if len(got) != 5 {
		t.Errorf("Full with MaxPages 5 sampled %d URLs, want <= 5", len(got))
	}
	if hasDup(got) {
		t.Errorf("result has duplicates: %v", got)
	}

	// Small site (fewer URLs than budget): Full returns everything, unchanged.
	small := groupByPattern([]string{"https://ex.com/", "https://ex.com/about"})
	if g := sample(small, ScrapeOptions{BaseURL: "https://ex.com", MaxPages: 50, Full: true}); len(g) != 2 {
		t.Errorf("Full on small site sampled %d URLs, want 2", len(g))
	}
}

func TestSampleIncludeExclude(t *testing.T) {
	groups := bigCorpus()

	inc := sample(groups, ScrapeOptions{
		BaseURL: "https://ex.com", MaxPages: 100, MaxPerPattern: 100,
		IncludePatterns: []string{"/blog/**"},
	})
	if len(inc) == 0 {
		t.Fatal("include /blog/** returned nothing")
	}
	for _, u := range inc {
		if !strings.HasPrefix(pathOf(u), "/blog/") {
			t.Errorf("include /blog/** leaked %s", u)
		}
	}

	exc := sample(groups, ScrapeOptions{
		BaseURL: "https://ex.com", MaxPages: 100, MaxPerPattern: 100,
		ExcludePatterns: []string{"/products/**"},
	})
	for _, u := range exc {
		if strings.HasPrefix(pathOf(u), "/products/") {
			t.Errorf("exclude /products/** leaked %s", u)
		}
	}
}

func TestSamplePriorityHomepageAndSections(t *testing.T) {
	groups := bigCorpus()
	got := sample(groups, ScrapeOptions{
		BaseURL: "https://ex.com", MaxPages: 4, MaxPerPattern: 8, SampleStrategy: SamplePriority,
	})
	if len(got) != 4 {
		t.Fatalf("priority sample = %v, want 4", got)
	}
	set := map[string]bool{}
	for _, u := range got {
		set[pathOf(u)] = true
	}
	if !set["/"] {
		t.Errorf("priority did not include homepage: %v", got)
	}
	// The two shallowest sections (about, contact) should be represented first.
	if !set["/about"] || !set["/contact"] {
		t.Errorf("priority missed top-level sections: %v", got)
	}
}

func TestSampleDeterministic(t *testing.T) {
	groups := bigCorpus()
	for _, strat := range []SampleStrategy{SampleRandom, SampleBalanced, SamplePriority} {
		opts := ScrapeOptions{BaseURL: "https://ex.com", MaxPages: 12, MaxPerPattern: 4, SampleStrategy: strat}
		a := sample(groups, opts)
		b := sample(groups, opts)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("[%s] not deterministic:\n a=%v\n b=%v", strat, a, b)
		}
	}
}

func TestGlobToRegex(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"/blog/*", "/blog/1", true},
		{"/blog/*", "/blog/1/post", false}, // * stays within a segment
		{"/blog/**", "/blog/1/post", true}, // ** crosses segments
		{"/products/*/detail", "/products/9/detail", true},
		{"/about", "/about", true},
		{"/about", "/about-us", false},
	}
	for _, c := range cases {
		if got := globToRegex(c.glob).MatchString(c.path); got != c.want {
			t.Errorf("globToRegex(%q).Match(%q) = %v, want %v", c.glob, c.path, got, c.want)
		}
	}
}

func hasDup(s []string) bool {
	seen := map[string]bool{}
	for _, x := range s {
		if seen[x] {
			return true
		}
		seen[x] = true
	}
	return false
}
