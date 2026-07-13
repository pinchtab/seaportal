package engine

import (
	"strings"
	"testing"
	"time"
)

func TestSanitizeHTML_ScriptBodyNotParsedAsMarkup(t *testing.T) {
	js := `for(var r=0;r<t.length;r++){if(a<b&&c>d){x="</div hidden>"}}`
	html := `<html><body><script>` + js + `</script><p>keep me</p></body></html>`

	out := sanitizeWithDeadline(t, html, 5*time.Second)

	if !strings.Contains(out, "keep me") {
		t.Errorf("visible content dropped:\n%s", out)
	}
	if !strings.Contains(out, js) {
		t.Errorf("script body was altered (parsed as markup):\n%s", out)
	}
}

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
