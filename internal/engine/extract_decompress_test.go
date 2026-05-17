package engine

import (
	"bytes"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestDecompressBody_Zstd_RoundTrip(t *testing.T) {
	input := []byte("hello world — zstd round-trip with multibyte: 日本語 / 中文 / العربية")
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if _, err := w.Write(input); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := decompressBody(buf.Bytes(), "zstd")
	if err != nil {
		t.Fatalf("decompressBody: %v", err)
	}
	if !bytes.Equal(got, input) {
		t.Errorf("round-trip mismatch:\n got: %q\nwant: %q", got, input)
	}
}

func TestDecompressBody_Zstd_Malformed(t *testing.T) {
	junk := []byte{0xff, 0xfe, 0x00, 0x00, 0xde, 0xad, 0xbe, 0xef}
	_, err := decompressBody(junk, "zstd")
	if err == nil {
		t.Fatalf("expected error on malformed zstd input, got nil")
	}
}

func TestDecompressBody_Zstd_Empty(t *testing.T) {
	// Empty input: zstd.NewReader returns a reader, ReadAll yields empty bytes.
	// Either no-error empty output OR an error is acceptable — must not crash.
	out, err := decompressBody([]byte{}, "zstd")
	if err == nil && len(out) != 0 {
		t.Errorf("expected empty output or error, got %d bytes (no err)", len(out))
	}
}
