package engine

import (
	"strings"
	"testing"
)

func TestCleanupMarkdown_StripCDATA(t *testing.T) {
	got := CleanupMarkdown("Hello <![CDATA[World]]>!")
	want := "Hello World!"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCleanupMarkdown_StripEntityEncodedCDATA(t *testing.T) {
	got := CleanupMarkdown("Hello &lt;![CDATA[World]]&gt;!")
	want := "Hello World!"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCleanupMarkdown_MultiLineCDATA(t *testing.T) {
	in := "before <![CDATA[line1\nline2]]> after"
	got := CleanupMarkdown(in)
	if !strings.Contains(got, "line1\nline2") {
		t.Fatalf("expected inner multi-line content preserved, got %q", got)
	}
	if strings.Contains(got, "CDATA") {
		t.Fatalf("CDATA wrapper not stripped: %q", got)
	}
}

func TestCleanupMarkdown_EmptyCDATA(t *testing.T) {
	got := CleanupMarkdown("before<![CDATA[]]>after")
	want := "beforeafter"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCleanupMarkdown_StripMarkdownEscapedCDATA(t *testing.T) {
	got := CleanupMarkdown(`Hello &lt;!\[CDATA\[World]]&gt;!`)
	want := "Hello World!"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCleanupMarkdown_PreservesNormalBrackets(t *testing.T) {
	in := "Use [link](url) and arr[0]"
	got := CleanupMarkdown(in)
	want := "Use [link](url) and arr[0]"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
