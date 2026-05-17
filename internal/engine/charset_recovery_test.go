package engine

import (
	"bytes"
	"strings"
	"testing"
)

// The recovery path lives inline in sniffAndDecode (see charset.go). These
// tests exercise it via that entry point — the only public-shaped surface —
// and assert the contract documented on bestCharset in todo-next.md:
//
//   - header matches meta → no recovery, header wins
//   - header lies AND decoded output is mojibake-y AND meta disagrees →
//     re-decode with meta and keep cleaner output
//   - no meta tag → no recovery possible, header decode passes through
//   - both decodes equally bad → keep header, don't introduce churn

// TestBestCharset_HeaderMatchesMeta_NoChange: header and meta both claim
// utf-8; recovery must not fire and the charset stays "utf-8".
func TestBestCharset_HeaderMatchesMeta_NoChange(t *testing.T) {
	body := []byte(`<!DOCTYPE html><html><head>` +
		`<meta charset="utf-8">` +
		`<title>Hello</title></head><body><p>Bonjour le monde.</p></body></html>`)
	decoded, cs, ok := sniffAndDecode(body, "text/html; charset=utf-8")
	if !ok {
		t.Fatalf("sniffAndDecode ok=false, want true")
	}
	if cs != "utf-8" {
		t.Fatalf("Charset = %q, want %q", cs, "utf-8")
	}
	if !bytes.Contains(decoded, []byte("Bonjour le monde.")) {
		t.Fatalf("decoded body missing expected text: %s", decoded)
	}
}

// TestBestCharset_RecoversOnMetaDisagreement: header lies (gb2312) over a
// latin-1 body whose <meta> says iso-8859-1; recovery must kick in.
func TestBestCharset_RecoversOnMetaDisagreement(t *testing.T) {
	// Build a ~1KB latin-1 body. 0xE9 = "é", 0xE8 = "è", 0xE0 = "à".
	var buf bytes.Buffer
	buf.WriteString(`<!DOCTYPE html><html lang="fr"><head>` +
		`<meta http-equiv="Content-Type" content="text/html; charset=ISO-8859-1">` +
		`<title>Test</title></head><body>`)
	// Repeat a Latin-1 paragraph until > 1KB.
	para := []byte("<p>Le caf")
	para = append(para, 0xE9) // é
	para = append(para, []byte(" fran")...)
	para = append(para, 0xE7) // ç
	para = append(para, []byte("ais pr")...)
	para = append(para, 0xE8) // è
	para = append(para, []byte("s de la Seine, Fran")...)
	para = append(para, 0xE7) // ç
	para = append(para, []byte("ois sert le caf")...)
	para = append(para, 0xE9) // é
	para = append(para, []byte(" noir ")...)
	para = append(para, 0xE0) // à
	para = append(para, []byte(" Paris.</p>\n")...)
	for buf.Len() < 1200 {
		buf.Write(para)
	}
	buf.WriteString("</body></html>")

	decoded, cs, ok := sniffAndDecode(buf.Bytes(), "text/html; charset=gb2312")
	if !ok {
		t.Fatalf("sniffAndDecode ok=false, want true")
	}
	if cs != "iso-8859-1" {
		t.Fatalf("Charset = %q, want recovered %q", cs, "iso-8859-1")
	}
	got := string(decoded)
	for _, want := range []string{"café", "français", "François"} {
		if !strings.Contains(got, want) {
			t.Errorf("recovered output missing %q", want)
		}
	}
	for _, bad := range []string{"鏰", "鑣", "鏾"} {
		if strings.Contains(got, bad) {
			t.Errorf("recovered output still contains mojibake marker %q", bad)
		}
	}
}

// TestBestCharset_NoMetaNoRecovery: no <meta> tag in the body, so recovery
// has nothing to fall back to. The header decode is kept verbatim, even
// when the result contains some mojibake.
func TestBestCharset_NoMetaNoRecovery(t *testing.T) {
	// A latin-1 body with NO <meta charset> declaration. Served as
	// gb2312, which decodes the high-byte sequences into CJK glyphs.
	var buf bytes.Buffer
	buf.WriteString("<html><body><p>")
	for i := 0; i < 50; i++ {
		buf.WriteByte(0xE9) // bare latin-1 "é"
		buf.WriteByte(0xE8) // bare latin-1 "è"
		buf.WriteString(" ")
	}
	buf.WriteString("</p></body></html>")

	_, cs, ok := sniffAndDecode(buf.Bytes(), "text/html; charset=gb2312")
	if !ok {
		t.Fatalf("sniffAndDecode ok=false, want true")
	}
	if cs != "gb2312" {
		t.Fatalf("Charset = %q, want header %q (no meta to recover from)", cs, "gb2312")
	}
}

// TestBestCharset_BothDecodesPoor_KeepsHeader: when neither the header
// charset nor the meta charset produces a cleaner decode, the header
// decode is retained — recovery only fires when the alternative is
// strictly better.
func TestBestCharset_BothDecodesPoor_KeepsHeader(t *testing.T) {
	// Body declares <meta charset="utf-8"> but contains stray raw bytes
	// outside ASCII that aren't valid UTF-8 AND aren't a sensible
	// latin-1 paragraph either. Header says iso-8859-1.
	// The header decode (latin-1) will be clean (every byte maps), so
	// recovery should NOT fire — the meta (utf-8) re-decode would
	// introduce U+FFFD and be WORSE, so the header wins.
	var buf bytes.Buffer
	buf.WriteString(`<!DOCTYPE html><html><head>` +
		`<meta charset="utf-8">` +
		`<title>t</title></head><body><p>`)
	// Random high bytes that latin-1 maps cleanly but utf-8 rejects.
	for i := 0; i < 200; i++ {
		buf.WriteByte(byte(0xC0 + (i % 32))) // 0xC0..0xDF — incomplete UTF-8 leads
	}
	buf.WriteString("</p></body></html>")

	_, cs, ok := sniffAndDecode(buf.Bytes(), "text/html; charset=iso-8859-1")
	if !ok {
		t.Fatalf("sniffAndDecode ok=false, want true")
	}
	// Header decode (latin-1) is cleaner than the meta (utf-8) re-decode,
	// so the header charset must be retained.
	if cs != "iso-8859-1" {
		t.Fatalf("Charset = %q, want header %q (meta re-decode no better)", cs, "iso-8859-1")
	}
}
