package engine

import (
	"bytes"
	"strings"
	"testing"
)

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

func TestBestCharset_RecoversOnMetaDisagreement(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`<!DOCTYPE html><html lang="fr"><head>` +
		`<meta http-equiv="Content-Type" content="text/html; charset=ISO-8859-1">` +
		`<title>Test</title></head><body>`)
	para := []byte("<p>Le caf")
	para = append(para, 0xE9)
	para = append(para, []byte(" fran")...)
	para = append(para, 0xE7)
	para = append(para, []byte("ais pr")...)
	para = append(para, 0xE8)
	para = append(para, []byte("s de la Seine, Fran")...)
	para = append(para, 0xE7)
	para = append(para, []byte("ois sert le caf")...)
	para = append(para, 0xE9)
	para = append(para, []byte(" noir ")...)
	para = append(para, 0xE0)
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

func TestBestCharset_NoMetaNoRecovery(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("<html><body><p>")
	for i := 0; i < 50; i++ {
		buf.WriteByte(0xE9)
		buf.WriteByte(0xE8)
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

func TestBestCharset_BothDecodesPoor_KeepsHeader(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`<!DOCTYPE html><html><head>` +
		`<meta charset="utf-8">` +
		`<title>t</title></head><body><p>`)
	for i := 0; i < 200; i++ {
		buf.WriteByte(byte(0xC0 + (i % 32)))
	}
	buf.WriteString("</p></body></html>")

	_, cs, ok := sniffAndDecode(buf.Bytes(), "text/html; charset=iso-8859-1")
	if !ok {
		t.Fatalf("sniffAndDecode ok=false, want true")
	}
	if cs != "iso-8859-1" {
		t.Fatalf("Charset = %q, want header %q (meta re-decode no better)", cs, "iso-8859-1")
	}
}
