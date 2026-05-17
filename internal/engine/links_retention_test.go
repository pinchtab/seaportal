package engine

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseLinkRetention_Roundtrip(t *testing.T) {
	cases := []struct {
		in   string
		want LinkRetention
	}{
		{"all", LinkRetentionAll},
		{"none", LinkRetentionNone},
		{"text", LinkRetentionText},
		{"footer", LinkRetentionFooter},
	}
	for _, c := range cases {
		got, err := ParseLinkRetention(c.in)
		if err != nil {
			t.Errorf("ParseLinkRetention(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseLinkRetention(%q) = %v, want %v", c.in, got, c.want)
		}
		if got.String() != c.in {
			t.Errorf("(%v).String() = %q, want %q", got, got.String(), c.in)
		}
	}

	if _, err := ParseLinkRetention("bogus"); err == nil {
		t.Errorf("ParseLinkRetention(\"bogus\"): expected error, got nil")
	}
}

func TestApplyLinkRetention_NoneRemovesLinks(t *testing.T) {
	got := applyLinkRetention("Visit [docs](https://x.com)", LinkRetentionNone)
	// Trailing space left where the link used to be is acceptable; we just
	// want to confirm the URL and bracketed text are gone.
	if strings.Contains(got, "docs") || strings.Contains(got, "x.com") || strings.Contains(got, "[") {
		t.Fatalf("none mode did not strip link: %q", got)
	}
	if !strings.HasPrefix(got, "Visit") {
		t.Fatalf("none mode dropped surrounding text: %q", got)
	}
}

func TestApplyLinkRetention_NoneCollapsesDoubleSpaces(t *testing.T) {
	got := applyLinkRetention("See [a](u) here.", LinkRetentionNone)
	if strings.Contains(got, "  ") {
		t.Fatalf("none mode left double space: %q", got)
	}
}

func TestApplyLinkRetention_TextKeepsTextDropsURL(t *testing.T) {
	got := applyLinkRetention("Visit [docs](https://x.com)", LinkRetentionText)
	want := "Visit docs"
	if got != want {
		t.Fatalf("text mode = %q, want %q", got, want)
	}
}

func TestApplyLinkRetention_AllNoOp(t *testing.T) {
	in := "Visit [docs](https://x.com) and [more](https://y.com)."
	got := applyLinkRetention(in, LinkRetentionAll)
	if got != in {
		t.Fatalf("all mode mutated input: %q -> %q", in, got)
	}
}

func TestApplyLinkRetention_FooterDelegatesToCitations(t *testing.T) {
	got := applyLinkRetention("See [a](u)", LinkRetentionFooter)
	if !strings.Contains(got, "⟨1⟩") {
		t.Fatalf("footer mode missing ⟨1⟩ marker: %q", got)
	}
	if !strings.Contains(got, "## References") {
		t.Fatalf("footer mode missing References section: %q", got)
	}
}

func TestApplyLinkRetention_PreservesCodeBlocks(t *testing.T) {
	src := "Before [x](u) after.\n\n```\nkeep [code](url) alone\n```\n\nInline `[ic](iu)` too."
	for _, mode := range []LinkRetention{LinkRetentionNone, LinkRetentionText, LinkRetentionFooter} {
		got := applyLinkRetention(src, mode)
		if !strings.Contains(got, "keep [code](url) alone") {
			t.Errorf("mode %v: fenced code rewritten: %q", mode, got)
		}
		if !strings.Contains(got, "`[ic](iu)`") {
			t.Errorf("mode %v: inline code rewritten: %q", mode, got)
		}
	}
}

func TestApplyLinkRetention_LeavesImagesAlone(t *testing.T) {
	src := "An image: ![alt](https://img/x.png) here."
	for _, mode := range []LinkRetention{LinkRetentionNone, LinkRetentionText, LinkRetentionFooter, LinkRetentionAll} {
		got := applyLinkRetention(src, mode)
		if !strings.Contains(got, "![alt](https://img/x.png)") {
			t.Errorf("mode %v: image syntax mutated: %q", mode, got)
		}
	}
}

// ── Integration: httptest-driven Options.LinkRetention plumbing ─────

func linkPage() string {
	prose := ""
	for i := 0; i < 20; i++ {
		prose += "This is a paragraph of sample prose used for extraction testing. "
	}
	return fmt.Sprintf(`<!doctype html><html><head><title>Links Test</title></head><body>
<article><h1>Links Test</h1>
<p>%s</p>
<p>Visit <a href="https://example.com/docs">our docs</a> for more info.</p>
<p>Also see <a href="https://example.com/api">the API</a>.</p>
</article></body></html>`, prose)
}

func serveLinkPage(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(linkPage()))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestExtract_LinksModeNone(t *testing.T) {
	srv := serveLinkPage(t)
	r := FromURLWithOptions(srv.URL+"/", Options{LinkRetention: LinkRetentionNone})
	if strings.Contains(r.Content, "](https://example.com") {
		t.Fatalf("none mode left link URLs in content:\n%s", r.Content)
	}
	if strings.Contains(r.Content, "[our docs]") {
		t.Fatalf("none mode left bracketed text in content:\n%s", r.Content)
	}
}

func TestExtract_LinksModeText(t *testing.T) {
	srv := serveLinkPage(t)
	r := FromURLWithOptions(srv.URL+"/", Options{LinkRetention: LinkRetentionText})
	if strings.Contains(r.Content, "https://example.com") {
		t.Fatalf("text mode left URL in content:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, "our docs") {
		t.Fatalf("text mode dropped link text:\n%s", r.Content)
	}
	if strings.Contains(r.Content, "[our docs]") {
		t.Fatalf("text mode left bracketed markup:\n%s", r.Content)
	}
}

func TestExtract_LinksModeFooter(t *testing.T) {
	srv := serveLinkPage(t)
	viaLinks := FromURLWithOptions(srv.URL+"/", Options{LinkRetention: LinkRetentionFooter})
	viaCitations := FromURLWithOptions(srv.URL+"/", Options{Citations: true})

	if !strings.Contains(viaLinks.Content, "## References") {
		t.Fatalf("footer mode missing References section:\n%s", viaLinks.Content)
	}
	if !strings.Contains(viaLinks.Content, "⟨1⟩") {
		t.Fatalf("footer mode missing ⟨1⟩ marker:\n%s", viaLinks.Content)
	}
	if viaLinks.Content != viaCitations.Content {
		t.Fatalf("back-compat broken: --links=footer differs from --citations\n--links:\n%s\n--citations:\n%s",
			viaLinks.Content, viaCitations.Content)
	}
}
