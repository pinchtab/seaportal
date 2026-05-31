package engine

import (
	"strings"
	"testing"
)

func TestCanonicalize_StripsUTM(t *testing.T) {
	got, err := CanonicalizeURL("https://example.com/post?utm_source=tw&utm_medium=x&id=42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://example.com/post?id=42" {
		t.Fatalf("got %q", got)
	}
}

func TestCanonicalize_StripsFragment(t *testing.T) {
	got, _ := CanonicalizeURL("https://example.com/post#comments")
	if got != "https://example.com/post" {
		t.Fatalf("got %q", got)
	}
}

func TestCanonicalize_LowercasesHost(t *testing.T) {
	got, _ := CanonicalizeURL("https://EXAMPLE.com/Path")
	if got != "https://example.com/Path" {
		t.Fatalf("got %q", got)
	}
}

func TestCanonicalize_RemovesDefaultPort(t *testing.T) {
	cases := map[string]string{
		"https://example.com:443/x":  "https://example.com/x",
		"http://example.com:80/x":    "http://example.com/x",
		"https://example.com:8443/x": "https://example.com:8443/x", // non-default kept
	}
	for in, want := range cases {
		got, _ := CanonicalizeURL(in)
		if got != want {
			t.Errorf("CanonicalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalize_SortsRemainingParams(t *testing.T) {
	got, _ := CanonicalizeURL("https://example.com/x?b=2&a=1&c=3")
	if got != "https://example.com/x?a=1&b=2&c=3" {
		t.Fatalf("got %q", got)
	}
}

func TestCanonicalize_CollapsesDuplicateSlashes(t *testing.T) {
	got, _ := CanonicalizeURL("https://example.com/foo//bar///baz")
	if got != "https://example.com/foo/bar/baz" {
		t.Fatalf("got %q", got)
	}
}

func TestCanonicalize_HandlesMalformedURL(t *testing.T) {
	in := "ht tp://bad url with spaces\x7f"
	got, err := CanonicalizeURL(in)
	if err == nil {
		t.Fatalf("expected error for malformed URL")
	}
	if got != in {
		t.Fatalf("expected input pass-through, got %q", got)
	}
}

func TestCanonicalize_CaseInsensitiveTrackingMatch(t *testing.T) {
	got, _ := CanonicalizeURL("https://example.com/x?UTM_Source=foo&id=1")
	if got != "https://example.com/x?id=1" {
		t.Fatalf("got %q", got)
	}
}

func TestCanonicalize_PrefixMatch(t *testing.T) {
	got, _ := CanonicalizeURL("https://example.com/x?_ga_ABC123=x&_gid_DEF=y&keep=1")
	if got != "https://example.com/x?keep=1" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveCanonicalLink_Absolute(t *testing.T) {
	html := `<html><head><link rel="canonical" href="https://example.com/canonical"></head></html>`
	got := ResolveCanonicalLink(html, "https://other.example.com/messy?x=1")
	if got != "https://example.com/canonical" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveCanonicalLink_Relative(t *testing.T) {
	html := `<html><head><link rel="canonical" href="/canonical-path"></head></html>`
	got := ResolveCanonicalLink(html, "https://example.com/messy?x=1")
	if got != "https://example.com/canonical-path" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveCanonicalLink_MissingReturnsEmpty(t *testing.T) {
	html := `<html><head><title>no canonical</title></head></html>`
	got := ResolveCanonicalLink(html, "https://example.com/")
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestResolveCanonicalLink_IgnoresJavascriptScheme(t *testing.T) {
	html := `<html><head><link rel="canonical" href="javascript:void(0)"></head></html>`
	got := ResolveCanonicalLink(html, "https://example.com/")
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestResolveCanonicalLink_HrefBeforeRel(t *testing.T) {
	html := `<link href="https://example.com/can" rel="canonical">`
	got := ResolveCanonicalLink(html, "https://example.com/")
	if got != "https://example.com/can" {
		t.Fatalf("got %q", got)
	}
}

func TestPickCanonical_PrefersHTMLLink(t *testing.T) {
	html := `<link rel="canonical" href="https://example.com/clean">`
	raw := "https://example.com/messy?utm_source=tw"
	got := PickCanonical(raw, html)
	if got != "https://example.com/clean" {
		t.Fatalf("got %q", got)
	}
}

func TestPickCanonical_FallsBackToAlgorithmic(t *testing.T) {
	raw := "https://example.com/post?utm_source=tw&id=42"
	got := PickCanonical(raw, "<html></html>")
	if got != "https://example.com/post?id=42" {
		t.Fatalf("got %q", got)
	}
}

func TestPickCanonical_NoChangeReturnsEmpty(t *testing.T) {
	raw := "https://example.com/post?id=42"
	got := PickCanonical(raw, "<html></html>")
	if got != "" {
		t.Fatalf("expected empty for already-clean URL, got %q", got)
	}
}

func TestResolveCanonicalLink_OutsideScanWindow(t *testing.T) {
	// Push the canonical link past the 4 KB window — must be ignored.
	pad := strings.Repeat(" ", 5000)
	html := "<html><head>" + pad + `<link rel="canonical" href="https://example.com/late"></head></html>`
	got := ResolveCanonicalLink(html, "https://example.com/")
	if got != "" {
		t.Fatalf("expected empty (outside scan window), got %q", got)
	}
}
