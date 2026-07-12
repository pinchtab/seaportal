// Package corpus loads the extraction-quality eval corpus
// (tests/eval/corpus.yaml). It is a benchmark/eval helper shared by
// cmd/seabench and the engine test suite; no extraction is performed here.
package corpus

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Entry describes a single fixture's extraction-quality expectations.
//
//   - Path:        repo-relative path to the HTML fixture.
//   - MustInclude: substrings that MUST appear in the extracted Markdown.
//   - MustExclude: substrings that MUST NOT survive extraction (chrome,
//     signup CTAs, layout debris, etc.).
//   - ExpectClass: canonical PageClass value
//     (static|ssr|hydrated|spa|dynamic|blocked).
//   - ExpectLang:  expected BCP-47 language tag; empty when undefined.
type Entry struct {
	Path        string   `yaml:"path"`
	MustInclude []string `yaml:"must_include"`
	MustExclude []string `yaml:"must_exclude"`
	ExpectClass string   `yaml:"expect_class"`
	ExpectLang  string   `yaml:"expect_lang"`
}

// Load reads and YAML-decodes a corpus file. Any read or parse error is
// wrapped for caller context.
func Load(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read corpus: %w", err)
	}
	var entries []Entry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse corpus: %w", err)
	}
	return entries, nil
}
