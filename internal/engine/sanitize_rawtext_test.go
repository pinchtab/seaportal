package engine

import (
	"strings"
	"testing"
	"time"
)

// A minified <script> body is full of `<`, `>`, and quotes that must not be
// tokenised as HTML. Before the raw-text fix this misparsed into pseudo-tags
// and the hidden-attribute regexes backtracked catastrophically — an effective
// hang on real news pages such as repubblica.it (script starts ~2 KB in).
func TestSanitizeHTML_ScriptBodyNotParsedAsMarkup(t *testing.T) {
	js := `for(var r=0;r<t.length;r++){if(a<b&&c>d){x="</div hidden>"}}`
	html := `<html><body><script>` + js + `</script><p>keep me</p></body></html>`

	out := sanitizeWithDeadline(t, html, 5*time.Second)

	if !strings.Contains(out, "keep me") {
		t.Errorf("visible content dropped:\n%s", out)
	}
	// The `hidden` / `<div>` living inside the JS string must not be treated
	// as a real hidden element — the script body survives verbatim.
	if !strings.Contains(out, js) {
		t.Errorf("script body was altered (parsed as markup):\n%s", out)
	}
}

// A large script-heavy document must sanitize in roughly linear time. This is
// the perf-regression guard for the O(n^2) blow-up: the huge timeout only
// fires if the pathological rescanning returns.
func TestSanitizeHTML_ScriptHeavyIsFast(t *testing.T) {
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 3000; i++ {
		b.WriteString(`<script>a<b;c>d;e="<span hidden>x</span>";f<g;h>i;</script>`)
		b.WriteString(`<p>para</p>`)
	}
	b.WriteString("</body></html>")
	in := b.String()

	start := time.Now()
	out := sanitizeWithDeadline(t, in, 5*time.Second)
	t.Logf("sanitized %d bytes in %v", len(in), time.Since(start))

	if !strings.Contains(out, "para") {
		t.Error("visible content dropped from script-heavy input")
	}
}

// sanitizeWithDeadline runs SanitizeHTML and fails the test if it does not
// return within d (guarding against a regression to the O(n^2) hang).
func sanitizeWithDeadline(t *testing.T, html string, d time.Duration) string {
	t.Helper()
	done := make(chan string, 1)
	go func() { done <- SanitizeHTML(html) }()
	select {
	case out := <-done:
		return out
	case <-time.After(d):
		t.Fatalf("SanitizeHTML did not complete within %v (%d-byte input)", d, len(html))
		return ""
	}
}
