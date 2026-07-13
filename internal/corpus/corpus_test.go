package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCorpusFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "corpus.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write corpus file: %v", err)
	}
	return path
}

func TestLoad_HappyPath(t *testing.T) {
	path := writeCorpusFile(t, `
- path: testdata/static/article.html
  must_include:
    - "headline one"
    - "second paragraph"
  must_exclude:
    - "Sign up now"
  expect_class: static
  expect_lang: en
- path: testdata/spa/app.html
  expect_class: spa
`)

	entries, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}

	first := entries[0]
	if first.Path != "testdata/static/article.html" {
		t.Errorf("Path = %q", first.Path)
	}
	if len(first.MustInclude) != 2 || first.MustInclude[0] != "headline one" {
		t.Errorf("MustInclude = %v", first.MustInclude)
	}
	if len(first.MustExclude) != 1 || first.MustExclude[0] != "Sign up now" {
		t.Errorf("MustExclude = %v", first.MustExclude)
	}
	if first.ExpectClass != "static" || first.ExpectLang != "en" {
		t.Errorf("ExpectClass/ExpectLang = %q/%q", first.ExpectClass, first.ExpectLang)
	}

	second := entries[1]
	if second.ExpectClass != "spa" {
		t.Errorf("second ExpectClass = %q", second.ExpectClass)
	}
	if second.MustInclude != nil || second.ExpectLang != "" {
		t.Errorf("omitted fields must stay zero-valued: %+v", second)
	}
}

func TestLoad_EmptyFileYieldsNoEntries(t *testing.T) {
	path := writeCorpusFile(t, "")
	entries, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("len(entries) = %d, want 0", len(entries))
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	path := writeCorpusFile(t, "- path: [unclosed\n  nonsense: {{")
	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse corpus") {
		t.Errorf("error should be wrapped with 'parse corpus': %v", err)
	}
}

func TestLoad_WrongShapeYAML(t *testing.T) {
	path := writeCorpusFile(t, "path: not-a-list\n")
	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected shape error, got nil")
	}
	if !strings.Contains(err.Error(), "parse corpus") {
		t.Errorf("error should be wrapped with 'parse corpus': %v", err)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatalf("expected read error, got nil")
	}
	if !strings.Contains(err.Error(), "read corpus") {
		t.Errorf("error should be wrapped with 'read corpus': %v", err)
	}
}
