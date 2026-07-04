package engine

import (
	"encoding/json"
	"testing"
)

// This file locks down the two algorithms most likely to regress silently:
// URL pattern grouping (ALP-003) and sampling (ALP-004). The per-feature test
// files cover the mechanics; here we assert the stronger byte-identical
// determinism guarantees and a single consolidated URL-shape table.

// TestGroupingShapesTable asserts the pattern derived for every URL shape called
// out in the spec, exercised as full URLs (not bare segments).
func TestGroupingShapesTable(t *testing.T) {
	cases := []struct {
		name, url, wantPattern string
	}{
		{"blog wildcard", "https://ex.com/blog/2026/my-post", "/blog/*/*"},
		{"product detail", "https://ex.com/products/12345/detail", "/products/*/detail"},
		{"dated path", "https://ex.com/news/2026-07-04/headline", "/news/*/headline"},
		{"uuid id", "https://ex.com/item/550e8400-e29b-41d4-a716-446655440000", "/item/*"},
		{"hex hash", "https://ex.com/asset/deadbeefcafe0", "/asset/*"},
		{"paginated query", "https://ex.com/blog?page=7", "/blog"},
		{"flat top-level", "https://ex.com/about", "/about"},
		{"root", "https://ex.com/", "/"},
		{"mixed alnum id", "https://ex.com/u/post123", "/u/*"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			groups := groupByPattern([]string{c.url})
			if len(groups) != 1 {
				t.Fatalf("got %d groups, want 1", len(groups))
			}
			if groups[0].Pattern != c.wantPattern {
				t.Errorf("pattern(%s) = %q, want %q", c.url, groups[0].Pattern, c.wantPattern)
			}
		})
	}
}

// permutations returns a few deterministic reorderings of urls (rotations +
// reverse) so the test itself introduces no randomness.
func permutations(urls []string) [][]string {
	out := [][]string{append([]string(nil), urls...)}
	rev := make([]string, len(urls))
	for i, u := range urls {
		rev[len(urls)-1-i] = u
	}
	out = append(out, rev)
	for shift := 1; shift < len(urls); shift++ {
		rot := append(append([]string(nil), urls[shift:]...), urls[:shift]...)
		out = append(out, rot)
	}
	return out
}

func TestGroupingByteIdenticalAcrossOrderings(t *testing.T) {
	urls := []string{
		"https://ex.com/", "https://ex.com/about", "https://ex.com/contact",
		"https://ex.com/blog/1/post", "https://ex.com/blog/2/post", "https://ex.com/blog/3/post",
		"https://ex.com/products/10/detail", "https://ex.com/products/20/detail",
		"https://ex.com/blog?page=1", "https://ex.com/blog?page=2",
	}
	var want []byte
	for i, perm := range permutations(urls) {
		got, err := json.Marshal(groupByPattern(perm))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if i == 0 {
			want = got
			continue
		}
		if string(got) != string(want) {
			t.Errorf("grouping not byte-identical for ordering %d:\n got  %s\n want %s", i, got, want)
		}
	}
}

func TestSamplingByteIdenticalAcrossRuns(t *testing.T) {
	groups := bigCorpus() // 1 home + 20 /blog/*/post + 15 /products/*/detail + 2 flat
	for _, strat := range []SampleStrategy{SampleRandom, SampleBalanced, SamplePriority} {
		opts := ScrapeOptions{BaseURL: "https://ex.com", MaxPages: 14, MaxPerPattern: 4, SampleStrategy: strat}
		first, err := json.Marshal(sample(groups, opts))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		for run := 0; run < 5; run++ {
			got, _ := json.Marshal(sample(groups, opts))
			if string(got) != string(first) {
				t.Errorf("[%s] sample not identical on run %d:\n got  %s\n first %s", strat, run, got, first)
			}
		}
	}
}

func TestSamplingCapsAllStrategies(t *testing.T) {
	groups := bigCorpus()
	const maxPages, maxPer = 9, 2
	for _, strat := range []SampleStrategy{SampleRandom, SampleBalanced, SamplePriority} {
		got := sample(groups, ScrapeOptions{
			BaseURL: "https://ex.com", MaxPages: maxPages, MaxPerPattern: maxPer, SampleStrategy: strat,
		})
		if len(got) > maxPages {
			t.Errorf("[%s] %d urls > MaxPages %d", strat, len(got), maxPages)
		}
		for pat, n := range patternCounts(got) {
			if n > maxPer {
				t.Errorf("[%s] pattern %s: %d > MaxPerPattern %d", strat, pat, n, maxPer)
			}
		}
	}
}

func TestSamplingIncludeExcludeExclusive(t *testing.T) {
	groups := bigCorpus()

	// Exclusively included: only /products/* survive.
	inc := sample(groups, ScrapeOptions{
		BaseURL: "https://ex.com", MaxPages: 100, MaxPerPattern: 100,
		IncludePatterns: []string{"/products/**"},
	})
	if len(inc) == 0 {
		t.Fatal("include /products/** yielded nothing")
	}
	for _, u := range inc {
		if got := patternForPath(pathOf(u)); got != "/products/*/detail" {
			t.Errorf("include leaked non-product %s (%s)", u, got)
		}
	}

	// Fully excluded: no /blog/* remain.
	exc := sample(groups, ScrapeOptions{
		BaseURL: "https://ex.com", MaxPages: 100, MaxPerPattern: 100,
		ExcludePatterns: []string{"/blog/**"},
	})
	for _, u := range exc {
		if patternForPath(pathOf(u)) == "/blog/*/post" {
			t.Errorf("exclude /blog/** leaked %s", u)
		}
	}
}
