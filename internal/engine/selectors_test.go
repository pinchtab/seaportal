package engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplySelectorOps_EmptyOpsNoOp(t *testing.T) {
	in := `<html><body><p>hi</p></body></html>`
	out, warns := applySelectorOps(in, "", "")
	if out != in {
		t.Fatalf("expected input unchanged, got %q", out)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %v", warns)
	}
}

func TestApplySelectorOps_SelectArticle(t *testing.T) {
	in := `<html><body><nav>NAVTEXT</nav><article>ARTICLE BODY</article></body></html>`
	out, warns := applySelectorOps(in, "article", "")
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if !strings.Contains(out, "ARTICLE BODY") {
		t.Fatalf("article body missing from output:\n%s", out)
	}
	if strings.Contains(out, "NAVTEXT") {
		t.Fatalf("nav text leaked into scoped output:\n%s", out)
	}
}

func TestApplySelectorOps_SelectMultiple(t *testing.T) {
	in := `<html><body>
<div class="a">AAA</div>
<div class="b">BBB</div>
<div class="c">CCC</div>
<div class="a">AAA2</div>
</body></html>`
	out, warns := applySelectorOps(in, ".a, .b", "")
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	for _, want := range []string{"AAA", "BBB", "AAA2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "CCC") {
		t.Fatalf("non-matching div leaked:\n%s", out)
	}
}

func TestApplySelectorOps_StripSingle(t *testing.T) {
	in := `<html><body><p>keep</p><div class="ads">ADTEXT</div></body></html>`
	out, warns := applySelectorOps(in, "", ".ads")
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if strings.Contains(out, "ADTEXT") {
		t.Fatalf("strip failed; output:\n%s", out)
	}
	if !strings.Contains(out, "keep") {
		t.Fatalf("kept content vanished:\n%s", out)
	}
}

func TestApplySelectorOps_StripMultiple(t *testing.T) {
	in := `<html><body>
<p>keep</p>
<div class="ads">ADTEXT</div>
<nav class="nav">NAVTEXT</nav>
</body></html>`
	out, warns := applySelectorOps(in, "", ".ads, .nav")
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if strings.Contains(out, "ADTEXT") || strings.Contains(out, "NAVTEXT") {
		t.Fatalf("strip failed; output:\n%s", out)
	}
	if !strings.Contains(out, "keep") {
		t.Fatalf("kept content vanished:\n%s", out)
	}
}

func TestApplySelectorOps_StripThenSelect(t *testing.T) {
	in := `<html><body>
<article><div class="ads">ADTEXT</div><p>BODY</p></article>
<nav>NAVTEXT</nav>
</body></html>`
	out, warns := applySelectorOps(in, "article", ".ads")
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if !strings.Contains(out, "BODY") {
		t.Fatalf("body missing:\n%s", out)
	}
	if strings.Contains(out, "ADTEXT") {
		t.Fatalf("strip didn't fire:\n%s", out)
	}
	if strings.Contains(out, "NAVTEXT") {
		t.Fatalf("select didn't scope:\n%s", out)
	}
}

func TestApplySelectorOps_InvalidSelectorWarns(t *testing.T) {
	in := `<html><body><p>keep</p></body></html>`
	out, warns := applySelectorOps(in, "", "!!!")
	if len(warns) == 0 {
		t.Fatalf("expected warning for invalid selector")
	}
	found := false
	for _, w := range warns {
		if strings.Contains(w, "invalid --strip selector") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected invalid-selector warning, got %v", warns)
	}
	if !strings.Contains(out, "keep") {
		t.Fatalf("content damaged:\n%s", out)
	}
}

func TestApplySelectorOps_NoMatchSelectWarns(t *testing.T) {
	in := `<html><body><p>keep</p></body></html>`
	out, warns := applySelectorOps(in, ".nonexistent", "")
	if len(warns) == 0 {
		t.Fatalf("expected warning for zero-match select")
	}
	found := false
	for _, w := range warns {
		if strings.Contains(w, "no match for --select") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected no-match warning, got %v", warns)
	}
	if out != in {
		t.Fatalf("expected input unchanged on zero match; got:\n%s", out)
	}
}

// --- Integration via httptest ---

func selectorIntegrationHTML() string {
	return `<!doctype html>
<html><head><meta charset="utf-8"><title>Selector Demo</title></head>
<body>
<nav>NAV LINKS top</nav>
<article>
  <h1>Article Heading</h1>
  <p>This is the real article body with enough words to clear readability density thresholds and stay in the output for the test to inspect downstream extraction results consistently every run.</p>
  <p>Second paragraph keeps prose long enough so go-readability cannot prune the article subtree under any heuristic — repeated content here keeps density bounded comfortably.</p>
  <div class="cookie-banner">COOKIEBANNERTEXT we use cookies please accept them all today</div>
</article>
<footer>FOOTER TEXT bottom</footer>
</body></html>`
}

func TestExtract_SelectFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(selectorIntegrationHTML()))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL+"/", Options{SelectCSS: "article"})
	if result.Error != "" {
		t.Fatalf("extract error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "Article Heading") {
		t.Fatalf("expected article heading in content:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "NAV LINKS") {
		t.Fatalf("nav text leaked into --select output:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "FOOTER TEXT") {
		t.Fatalf("footer text leaked into --select output:\n%s", result.Content)
	}
}

func TestExtract_StripFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(selectorIntegrationHTML()))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL+"/", Options{StripCSS: ".cookie-banner"})
	if result.Error != "" {
		t.Fatalf("extract error: %s", result.Error)
	}
	if strings.Contains(result.Content, "COOKIEBANNERTEXT") {
		t.Fatalf("cookie banner not stripped:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "real article body") {
		t.Fatalf("article body missing post-strip:\n%s", result.Content)
	}
}

func TestExtract_SelectAndStripCombined(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(selectorIntegrationHTML()))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL+"/", Options{SelectCSS: "article", StripCSS: ".cookie-banner"})
	if result.Error != "" {
		t.Fatalf("extract error: %s", result.Error)
	}
	if strings.Contains(result.Content, "COOKIEBANNERTEXT") {
		t.Fatalf("strip failed:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "NAV LINKS") || strings.Contains(result.Content, "FOOTER TEXT") {
		t.Fatalf("select failed to scope:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "real article body") {
		t.Fatalf("article body missing:\n%s", result.Content)
	}
}
