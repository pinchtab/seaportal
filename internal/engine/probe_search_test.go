package engine

import (
	"strings"
	"testing"
)

func TestLooksLikeSearchURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://edition.cnn.com/search?q=hantavirus", true},
		{"https://duckduckgo.com/?q=hantavirus", true},
		{"https://html.duckduckgo.com/html/?q=hantavirus", true},
		{"https://en.wikipedia.org/w/index.php?search=hantavirus", true},
		{"https://www.repubblica.it/ricerca/?query=hantavirus", true},
		{"https://example.com/SEARCH?foo=bar", true},
		{"https://example.com/srp/listing", true},
		{"https://example.com/about", false},
		{"https://en.wikipedia.org/wiki/Hantavirus", false},
		{"https://example.com/", false},
		{"https://example.com/posts/django", false},
	}
	for _, c := range cases {
		got := looksLikeSearchURL(c.url)
		if got != c.want {
			t.Errorf("looksLikeSearchURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestLooksLikeSearchResults(t *testing.T) {
	linkList := "# Stuff\n\n" +
		"- [One](https://a.example/1)\n" +
		"- [Two](https://a.example/2)\n" +
		"- [Three](https://a.example/3)\n"
	cases := []struct {
		name string
		md   string
		want bool
	}{
		{"three link list items", linkList, true},
		{"results heading", "# Search Results\n\nSome prose.\n", true},
		{"numeric N results", "About 1,234 results found.\n", true},
		{"italian risultati", "Trovati 42 risultati per la ricerca.\n", true},
		{"chrome only", "# Welcome\n\nSign in. Menu. Footer.\n", false},
		{"single link", "- [One](https://a.example/1)\n", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		got := looksLikeSearchResults(c.md)
		if got != c.want {
			t.Errorf("%s: looksLikeSearchResults = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestProbeSearch_CNNStyleEmptyShell(t *testing.T) {
	r := &Result{
		URL:     "https://example.com/search?q=foo",
		Content: "# Search\n",
		Length:  200,
		Profile: PageProfile{Class: PageSSR, Outcome: OutcomeExtract, Reasons: []string{"high-confidence", "ssr-markers-present"}},
	}
	applyProbeSearchOverride(r, Options{ProbeSearch: true})
	if r.Profile.Outcome != OutcomeNeedsBrowser {
		t.Fatalf("outcome = %q, want needs-browser", r.Profile.Outcome)
	}
	if !containsReason(r.Profile.Reasons, "client-rendered-search") {
		t.Fatalf("reasons %v missing client-rendered-search", r.Profile.Reasons)
	}
	if !r.Validation.NeedsBrowser {
		t.Fatalf("Validation.NeedsBrowser should be set")
	}
	if r.Content != "# Search\n" {
		t.Fatalf("Content should not be mutated")
	}
}

func TestProbeSearch_DDGHtmlHasResults(t *testing.T) {
	body := "# Results\n\n" +
		"- [hantavirus overview](https://a.example/1)\n" +
		"- [hantavirus symptoms](https://a.example/2)\n" +
		"- [hantavirus outbreak](https://a.example/3)\n" +
		"- [hantavirus prevention](https://a.example/4)\n" +
		"- [hantavirus history](https://a.example/5)\n" +
		strings.Repeat("Hantavirus is a viral disease. ", 30)
	r := &Result{
		URL:     "https://html.duckduckgo.com/html/?q=hantavirus",
		Content: body,
		Length:  len(body),
		Profile: PageProfile{Class: PageSSR, Outcome: OutcomeExtract},
	}
	applyProbeSearchOverride(r, Options{ProbeSearch: true})
	if r.Profile.Outcome != OutcomeExtract {
		t.Fatalf("outcome = %q, want extract (unchanged)", r.Profile.Outcome)
	}
	if containsReason(r.Profile.Reasons, "client-rendered-search") {
		t.Fatalf("should not append client-rendered-search")
	}
}

func TestProbeSearch_NonSearchURLNoOp(t *testing.T) {
	r := &Result{
		URL:     "https://example.com/about",
		Content: "",
		Length:  0,
		Profile: PageProfile{Class: PageSSR, Outcome: OutcomeExtract},
	}
	applyProbeSearchOverride(r, Options{ProbeSearch: true})
	if r.Profile.Outcome != OutcomeExtract {
		t.Fatalf("outcome = %q, want extract (non-search URL)", r.Profile.Outcome)
	}
}

func TestProbeSearch_FlagOffNoOp(t *testing.T) {
	r := &Result{
		URL:     "https://example.com/search?q=foo",
		Content: "# Search\n",
		Length:  200,
		Profile: PageProfile{Class: PageSSR, Outcome: OutcomeExtract},
	}
	applyProbeSearchOverride(r, Options{ProbeSearch: false})
	if r.Profile.Outcome != OutcomeExtract {
		t.Fatalf("outcome = %q, want extract when flag is off", r.Profile.Outcome)
	}
	if containsReason(r.Profile.Reasons, "client-rendered-search") {
		t.Fatalf("should not append client-rendered-search when flag is off")
	}
}

func TestProbeSearch_PreservesSpaEscalate(t *testing.T) {
	r := &Result{
		URL:     "https://example.com/search?q=foo",
		Content: "",
		Length:  100,
		Profile: PageProfile{Class: PageSPA, Outcome: OutcomeNeedsBrowser, Reasons: []string{"spa-signal:react-root"}},
	}
	applyProbeSearchOverride(r, Options{ProbeSearch: true})
	if r.Profile.Class != PageSPA {
		t.Fatalf("class changed to %q", r.Profile.Class)
	}
	if containsReason(r.Profile.Reasons, "client-rendered-search") {
		t.Fatalf("should not add client-rendered-search to already-escalated SPA page; reasons=%v", r.Profile.Reasons)
	}
}

func containsReason(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
