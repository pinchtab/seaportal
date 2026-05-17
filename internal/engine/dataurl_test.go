package engine

import (
	"strings"
	"testing"
)

func TestParseDataURL_PlainText(t *testing.T) {
	mime, body, err := parseDataURL("data:text/plain,hello%20world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "text/plain" {
		t.Errorf("mime = %q, want text/plain", mime)
	}
	if string(body) != "hello world" {
		t.Errorf("body = %q, want %q", string(body), "hello world")
	}
}

func TestParseDataURL_HTMLInline(t *testing.T) {
	mime, body, err := parseDataURL("data:text/html,<h1>hi</h1>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "text/html" {
		t.Errorf("mime = %q, want text/html", mime)
	}
	if string(body) != "<h1>hi</h1>" {
		t.Errorf("body = %q, want %q", string(body), "<h1>hi</h1>")
	}
}

func TestParseDataURL_Base64(t *testing.T) {
	mime, body, err := parseDataURL("data:text/html;base64,PGgxPmhpPC9oMT4=")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "text/html" {
		t.Errorf("mime = %q, want text/html", mime)
	}
	if string(body) != "<h1>hi</h1>" {
		t.Errorf("body = %q, want %q", string(body), "<h1>hi</h1>")
	}
}

func TestParseDataURL_EmptyMime_DefaultsTextPlain(t *testing.T) {
	mime, body, err := parseDataURL("data:,hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "text/plain" {
		t.Errorf("mime = %q, want text/plain (RFC default)", mime)
	}
	if string(body) != "hello" {
		t.Errorf("body = %q, want %q", string(body), "hello")
	}
}

func TestParseDataURL_MissingComma(t *testing.T) {
	_, _, err := parseDataURL("data:text/html<h1>hi</h1>")
	if err == nil {
		t.Fatal("expected error for missing comma, got nil")
	}
}

func TestParseDataURL_BadBase64(t *testing.T) {
	_, _, err := parseDataURL("data:text/html;base64,!!!not-base64!!!")
	if err == nil {
		t.Fatal("expected error for malformed base64, got nil")
	}
}

func TestExtract_DataURL_HTML(t *testing.T) {
	res := FromURLWithOptions("data:text/html,<h1>Hello World</h1>", Options{})
	if res.Error != "" {
		t.Fatalf("unexpected error: %q", res.Error)
	}
	if !strings.Contains(res.Content, "Hello World") {
		t.Fatalf("Content missing 'Hello World'; got %q", res.Content)
	}
}

func TestExtract_DataURL_Binary_RejectedCleanly(t *testing.T) {
	// Use a valid base64 payload so the mime gate (not the decoder) is what
	// rejects this — that's the behaviour we're locking in.
	res := FromURLWithOptions("data:application/pdf;base64,JVBERi0=", Options{})
	if res.Error == "" {
		t.Fatal("expected error for unsupported binary mime, got empty")
	}
	if !strings.Contains(res.Error, "not supported") {
		t.Errorf("expected error to mention 'not supported'; got %q", res.Error)
	}
}
