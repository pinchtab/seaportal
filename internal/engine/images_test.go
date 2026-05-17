package engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractImages_BasicSrcAndAlt(t *testing.T) {
	html := `<html><body><img src="/foo.png" alt="Foo"></body></html>`
	imgs := ExtractImages(html, "https://example.com/")
	if len(imgs) != 1 {
		t.Fatalf("len=%d, want 1: %#v", len(imgs), imgs)
	}
	if got, want := imgs[0].Src, "https://example.com/foo.png"; got != want {
		t.Errorf("Src = %q, want %q", got, want)
	}
	if got, want := imgs[0].Alt, "Foo"; got != want {
		t.Errorf("Alt = %q, want %q", got, want)
	}
}

func TestExtractImages_ResolvesRelative(t *testing.T) {
	html := `<html><body>
		<img src="/abs.png">
		<img src="relative.png">
		<img src="../up.png">
	</body></html>`
	imgs := ExtractImages(html, "https://example.com/dir/page")

	wantSrcs := []string{
		"https://example.com/abs.png",
		"https://example.com/dir/relative.png",
		"https://example.com/up.png",
	}
	if len(imgs) != len(wantSrcs) {
		t.Fatalf("len=%d, want %d: %#v", len(imgs), len(wantSrcs), imgs)
	}
	for i, want := range wantSrcs {
		if imgs[i].Src != want {
			t.Errorf("imgs[%d].Src = %q, want %q", i, imgs[i].Src, want)
		}
	}
}

func TestExtractImages_PreservesSrcset(t *testing.T) {
	html := `<html><body><img src="/a.png" srcset="/a.png 1x, /a@2x.png 2x"></body></html>`
	imgs := ExtractImages(html, "https://example.com/")
	if len(imgs) != 1 {
		t.Fatalf("len=%d, want 1", len(imgs))
	}
	if got, want := imgs[0].Srcset, "/a.png 1x, /a@2x.png 2x"; got != want {
		t.Errorf("Srcset = %q, want %q", got, want)
	}
}

func TestExtractImages_PreservesTitle(t *testing.T) {
	html := `<html><body><img src="/a.png" title="Tooltip text"></body></html>`
	imgs := ExtractImages(html, "https://example.com/")
	if len(imgs) != 1 {
		t.Fatalf("len=%d, want 1", len(imgs))
	}
	if got, want := imgs[0].Title, "Tooltip text"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
}

func TestExtractImages_SkipsDataURLs(t *testing.T) {
	html := `<html><body>
		<img src="data:image/png;base64,iVBORw0KGgo=" alt="inline">
		<img src="/keep.png" alt="k">
	</body></html>`
	imgs := ExtractImages(html, "https://example.com/")
	if len(imgs) != 1 || !strings.HasSuffix(imgs[0].Src, "/keep.png") {
		t.Fatalf("unexpected images: %#v", imgs)
	}
}

func TestExtractImages_SkipsEmptySrc(t *testing.T) {
	html := `<html><body>
		<img src="" alt="empty">
		<img alt="no-src">
		<img src="   " alt="whitespace">
		<img src="/keep.png" alt="k">
	</body></html>`
	imgs := ExtractImages(html, "https://example.com/")
	if len(imgs) != 1 || !strings.HasSuffix(imgs[0].Src, "/keep.png") {
		t.Fatalf("unexpected images: %#v", imgs)
	}
}

func TestExtractImages_DedupesBySrc(t *testing.T) {
	html := `<html><body>
		<img src="/logo.png" alt="Logo">
		<img src="/logo.png" alt="Logo">
		<img src="/logo.png" alt="Different alt">
	</body></html>`
	imgs := ExtractImages(html, "https://example.com/")
	if len(imgs) != 1 {
		t.Fatalf("len=%d, want 1: %#v", len(imgs), imgs)
	}
	if imgs[0].Alt != "Logo" {
		t.Errorf("Alt = %q, want Logo (first occurrence wins)", imgs[0].Alt)
	}
}

func TestExtractImages_TruncatesLongAlt(t *testing.T) {
	long := strings.Repeat("a", 300)
	html := `<html><body><img src="/foo.png" alt="` + long + `"></body></html>`
	imgs := ExtractImages(html, "https://example.com/")
	if len(imgs) != 1 {
		t.Fatalf("len=%d, want 1", len(imgs))
	}
	runes := []rune(imgs[0].Alt)
	if len(runes) != 201 {
		t.Fatalf("rune len = %d, want 201 (200 + ellipsis)", len(runes))
	}
	if runes[200] != '…' {
		t.Errorf("last rune = %q, want …", runes[200])
	}
}

func TestExtractImages_IgnoresScriptStyleSubtrees(t *testing.T) {
	html := `<html><body>
		<img src="/visible.png" alt="v">
		<script><img src="/in-script.png" alt="s"></script>
		<style><img src="/in-style.png" alt="st"></style>
	</body></html>`
	imgs := ExtractImages(html, "https://example.com/")
	if len(imgs) != 1 {
		t.Fatalf("len=%d, want 1: %#v", len(imgs), imgs)
	}
	if !strings.HasSuffix(imgs[0].Src, "/visible.png") {
		t.Errorf("Src = %q, want /visible.png", imgs[0].Src)
	}
}

// imagesTestProse provides enough body text to keep readability happy.
var imagesTestProse = strings.Repeat("This page exists to exercise the image extractor end-to-end with realistic text. ", 10)

// imagesTestPage has 4 <img> elements:
//   - /hero.png (kept)
//   - data: URL (skipped)
//   - /thumb.png (kept)
//   - /hero.png duplicate (deduped)
//
// Expected: 2 entries in Images.
var imagesTestPage = `<!doctype html><html><head><title>Images Test</title></head><body>
<article><h1>Images Test</h1>
<p>` + imagesTestProse + `</p>
<img src="/hero.png" alt="Hero" srcset="/hero.png 1x, /hero@2x.png 2x">
<img src="data:image/png;base64,iVBORw0KGgo=" alt="inline">
<img src="/thumb.png" alt="Thumb" title="Thumbnail">
<img src="/hero.png" alt="Hero again">
</article></body></html>`

func TestExtract_ImagesFlagOn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(imagesTestPage))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL+"/", Options{WithImages: true})
	if len(result.Images) != 2 {
		t.Fatalf("len(Images) = %d, want 2: %#v", len(result.Images), result.Images)
	}

	if !strings.HasPrefix(result.Images[0].Src, srv.URL) {
		t.Errorf("Images[0].Src = %q, want prefix %q", result.Images[0].Src, srv.URL)
	}
	if !strings.HasSuffix(result.Images[0].Src, "/hero.png") {
		t.Errorf("Images[0].Src = %q, want suffix /hero.png", result.Images[0].Src)
	}
	if result.Images[0].Alt != "Hero" {
		t.Errorf("Images[0].Alt = %q, want Hero (first occurrence wins)", result.Images[0].Alt)
	}
	if result.Images[0].Srcset != "/hero.png 1x, /hero@2x.png 2x" {
		t.Errorf("Images[0].Srcset = %q, want preserved verbatim", result.Images[0].Srcset)
	}

	if !strings.HasSuffix(result.Images[1].Src, "/thumb.png") {
		t.Errorf("Images[1].Src = %q, want suffix /thumb.png", result.Images[1].Src)
	}
	if result.Images[1].Title != "Thumbnail" {
		t.Errorf("Images[1].Title = %q, want Thumbnail", result.Images[1].Title)
	}
}

func TestExtract_ImagesFlagOffDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(imagesTestPage))
	}))
	defer srv.Close()

	result := FromURL(srv.URL + "/")
	if result.Images != nil {
		t.Fatalf("Images = %#v, want nil when --with-images flag is off", result.Images)
	}
}
