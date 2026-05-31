package engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConvertCitations_BasicInlineLink(t *testing.T) {
	in := "See [docs](https://x.com/d) for more."
	got := ConvertLinksToCitations(in)
	if !strings.Contains(got, "See docs ⟨1⟩ for more.") {
		t.Fatalf("missing inline marker; got:\n%s", got)
	}
	if !strings.Contains(got, "## References") {
		t.Fatalf("missing References section; got:\n%s", got)
	}
	if !strings.Contains(got, "1. <https://x.com/d>") {
		t.Fatalf("missing reference entry; got:\n%s", got)
	}
}

func TestConvertCitations_DedupesSameURL(t *testing.T) {
	in := "[foo](https://x.com) and again [foo](https://x.com)."
	got := ConvertLinksToCitations(in)
	if strings.Count(got, "⟨1⟩") != 2 {
		t.Fatalf("want two ⟨1⟩ markers; got:\n%s", got)
	}
	if strings.Contains(got, "⟨2⟩") {
		t.Fatalf("dedupe failed — saw ⟨2⟩; got:\n%s", got)
	}
	if strings.Count(got, "1. <https://x.com>") != 1 {
		t.Fatalf("expected single reference entry; got:\n%s", got)
	}
}

func TestConvertCitations_NumbersInDocumentOrder(t *testing.T) {
	in := "[a](https://1.example) [b](https://2.example) [c](https://3.example)"
	got := ConvertLinksToCitations(in)
	want := "a ⟨1⟩ b ⟨2⟩ c ⟨3⟩"
	if !strings.Contains(got, want) {
		t.Fatalf("want %q in output; got:\n%s", want, got)
	}
	// Reference order must match.
	idx1 := strings.Index(got, "1. <https://1.example>")
	idx2 := strings.Index(got, "2. <https://2.example>")
	idx3 := strings.Index(got, "3. <https://3.example>")
	if idx1 < 0 || idx2 < 0 || idx3 < 0 || idx1 >= idx2 || idx2 >= idx3 {
		t.Fatalf("references out of order; got:\n%s", got)
	}
}

func TestConvertCitations_PreservesCodeBlockLinks(t *testing.T) {
	in := "Prose [real](https://real.example).\n\n```\n[fake](http://fake.example)\n```\n"
	got := ConvertLinksToCitations(in)
	if !strings.Contains(got, "[fake](http://fake.example)") {
		t.Fatalf("fenced-code link was modified; got:\n%s", got)
	}
	if strings.Contains(got, "<http://fake.example>") {
		t.Fatalf("fenced-code URL leaked into References; got:\n%s", got)
	}
	if !strings.Contains(got, "real ⟨1⟩") {
		t.Fatalf("prose link not converted; got:\n%s", got)
	}
}

func TestConvertCitations_PreservesInlineCodeBrackets(t *testing.T) {
	in := "Use `[link](url)` to define links."
	got := ConvertLinksToCitations(in)
	if got != in {
		t.Fatalf("inline-code link was touched; got:\n%s\nwant:\n%s", got, in)
	}
}

func TestConvertCitations_LeavesImagesAlone(t *testing.T) {
	in := "Look: ![alt](https://example.com/img.png) end."
	got := ConvertLinksToCitations(in)
	if got != in {
		t.Fatalf("image syntax was modified; got:\n%s", got)
	}
	if strings.Contains(got, "## References") {
		t.Fatalf("image produced References section; got:\n%s", got)
	}
}

func TestConvertCitations_NoLinksReturnsInputUnchanged(t *testing.T) {
	in := "Plain prose with no links at all."
	got := ConvertLinksToCitations(in)
	if got != in {
		t.Fatalf("input was modified; got:\n%s", got)
	}
	if strings.Contains(got, "## References") {
		t.Fatalf("References section appended to link-free input")
	}
}

func TestConvertCitations_HandlesMultipleLinksOnSameLine(t *testing.T) {
	in := "[a](https://u1.example) and [b](https://u2.example)"
	got := ConvertLinksToCitations(in)
	if !strings.Contains(got, "a ⟨1⟩ and b ⟨2⟩") {
		t.Fatalf("multi-link rewrite failed; got:\n%s", got)
	}
}

func TestConvertCitations_HandlesNestedEmphasis(t *testing.T) {
	in := "*[link](https://emp.example)*"
	got := ConvertLinksToCitations(in)
	if !strings.Contains(got, "*link ⟨1⟩*") {
		t.Fatalf("emphasis-wrapped link not converted; got:\n%s", got)
	}
}

// --- Integration ---

func citationsProse() string {
	// Long enough prose to ensure readability picks the article.
	s := "This is a sufficiently long paragraph of prose to satisfy the readability extractor and ensure the article body is detected as the primary content of the page. "
	return strings.Repeat(s, 6)
}

func citationsHTML() string {
	return `<!doctype html><html><head><title>Citations Page</title></head><body>
<article>
<h1>Citations Page</h1>
<p>` + citationsProse() + `</p>
<p>Read <a href="https://one.example/a">one</a>, then <a href="https://two.example/b">two</a>, finally <a href="https://three.example/c">three</a>.</p>
<p>` + citationsProse() + `</p>
</article>
</body></html>`
}

func TestExtract_CitationsFlagOn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(citationsHTML()))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL+"/", Options{Citations: true})
	if result.Error != "" {
		t.Fatalf("extract error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "## References") {
		t.Fatalf("expected ## References section; got:\n%s", result.Content)
	}
	for _, marker := range []string{"⟨1⟩", "⟨2⟩", "⟨3⟩"} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("missing marker %s; got:\n%s", marker, result.Content)
		}
	}
	if result.Length != len(result.Content) {
		t.Fatalf("Length=%d, len(Content)=%d — Length not refreshed", result.Length, len(result.Content))
	}
}

func TestExtract_CitationsFlagOffDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(citationsHTML()))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL+"/", Options{})
	if result.Error != "" {
		t.Fatalf("extract error: %s", result.Error)
	}
	if strings.Contains(result.Content, "⟨") {
		t.Fatalf("citation marker present with flag off; got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "## References") {
		t.Fatalf("References section present with flag off; got:\n%s", result.Content)
	}
}
