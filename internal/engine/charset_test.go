package engine

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
)

func TestDetectCharset_FromBOM(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		want string
	}{
		{"utf-8", append([]byte{0xEF, 0xBB, 0xBF}, []byte("<html></html>")...), "utf-8"},
		{"utf-16le", append([]byte{0xFF, 0xFE}, 'h', 0x00), "utf-16le"},
		{"utf-16be", append([]byte{0xFE, 0xFF}, 0x00, 'h'), "utf-16be"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectCharset(tc.body, ""); got != tc.want {
				t.Fatalf("detectCharset = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetectCharset_FromHTTPHeader(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		want        string
	}{
		{"iso-8859-1", "text/html; charset=iso-8859-1", "iso-8859-1"},
		{"quoted utf-8", `text/html; charset="utf-8"`, "utf-8"},
		{"malformed no charset", "text/html;", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectCharset([]byte("<html></html>"), tc.contentType); got != tc.want {
				t.Fatalf("detectCharset = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetectCharset_FromMetaCharset(t *testing.T) {
	body := []byte(`<!DOCTYPE html><html><head><meta charset="Shift_JIS"><title>x</title></head></html>`)
	if got := detectCharset(body, ""); got != "shift_jis" {
		t.Fatalf("detectCharset = %q, want shift_jis", got)
	}
}

func TestDetectCharset_FromMetaHttpEquiv(t *testing.T) {
	body := []byte(`<!DOCTYPE html><html><head><meta http-equiv="Content-Type" content="text/html; charset=GB2312"></head></html>`)
	if got := detectCharset(body, ""); got != "gb2312" {
		t.Fatalf("detectCharset = %q, want gb2312", got)
	}
}

func TestDetectCharset_DefaultUTF8(t *testing.T) {
	body := []byte(`<!DOCTYPE html><html><head><title>x</title></head><body><p>hello</p></body></html>`)
	if got := detectCharset(body, "text/html"); got != "" {
		t.Fatalf("detectCharset = %q, want empty", got)
	}
}

func TestDetectCharset_PriorityOrder(t *testing.T) {
	// Header says iso-8859-1; meta says utf-8. Header must win.
	body := []byte(`<!DOCTYPE html><html><head><meta charset="utf-8"></head></html>`)
	if got := detectCharset(body, "text/html; charset=iso-8859-1"); got != "iso-8859-1" {
		t.Fatalf("detectCharset = %q, want iso-8859-1 (header wins)", got)
	}
}

func TestDecodeBytes_Latin1Roundtrip(t *testing.T) {
	src := "Café à Paris ç"
	encoded, err := charmap.ISO8859_1.NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Sanity: encoded bytes contain Latin-1 0xE9 for é, not the UTF-8 0xC3 0xA9.
	if !bytes.Contains(encoded, []byte{0xE9}) {
		t.Fatalf("expected Latin-1 byte 0xE9 in encoded input")
	}
	decoded, err := decodeBytes(encoded, "iso-8859-1")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != src {
		t.Fatalf("decode mismatch: got %q want %q", string(decoded), src)
	}
}

func TestDecodeBytes_ShiftJIS(t *testing.T) {
	src := "日本語"
	encoded, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeBytes(encoded, "shift_jis")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != src {
		t.Fatalf("decode mismatch: got %q want %q", string(decoded), src)
	}
}

func TestDecodeBytes_UnknownEncodingReturnsInput(t *testing.T) {
	in := []byte("hello world")
	out, err := decodeBytes(in, "bogus-99")
	if err == nil {
		t.Fatalf("expected error for unknown charset")
	}
	if !bytes.Equal(in, out) {
		t.Fatalf("expected input returned unchanged, got %q", string(out))
	}
}

func TestDecodeBytes_UTF8NoOp(t *testing.T) {
	in := []byte("hello é world 日本")
	out, err := decodeBytes(in, "utf-8")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(in, out) {
		t.Fatalf("expected unchanged, got %q", string(out))
	}
}

func TestSniffAndDecode_BOMStrippedAfterDetection(t *testing.T) {
	in := append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello world")...)
	out, cs, ok := sniffAndDecode(in, "")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if cs != "utf-8" {
		t.Fatalf("charset = %q, want utf-8", cs)
	}
	if bytes.HasPrefix(out, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("BOM not stripped: %x", out[:3])
	}
	if string(out) != "hello world" {
		t.Fatalf("got %q", string(out))
	}
}

func TestIsCharsetSniffableContentType(t *testing.T) {
	cases := map[string]bool{
		"text/html; charset=utf-8": true,
		"text/plain":               true,
		"application/xhtml+xml":    true,
		"":                         true,
		"application/pdf":          false,
		"image/png":                false,
		"application/json":         false,
		"application/octet-stream": false,
	}
	for ct, want := range cases {
		if got := isCharsetSniffableContentType(ct); got != want {
			t.Errorf("isCharsetSniffableContentType(%q) = %v, want %v", ct, got, want)
		}
	}
	// Spot check meaningful case-insensitivity.
	if !isCharsetSniffableContentType(strings.ToUpper("Text/HTML")) {
		t.Errorf("case folding broken")
	}
}
