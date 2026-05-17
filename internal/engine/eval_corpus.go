// Package engine — eval corpus loader.
//
// CorpusEntry mirrors the schema in tests/eval/corpus.yaml. LoadCorpus parses
// the YAML file and returns the list of entries; no extraction is performed
// here. The accompanying scorer/runner is a separate backlog item.
package engine

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// CorpusEntry describes a single fixture's extraction-quality expectations.
//
//   - Path:        repo-relative path to the HTML fixture.
//   - MustInclude: substrings that MUST appear in the extracted Markdown.
//   - MustExclude: substrings that MUST NOT survive extraction (chrome,
//     signup CTAs, layout debris, etc.).
//   - ExpectClass: canonical PageClass value
//     (static|ssr|hydrated|spa|dynamic|blocked).
//   - ExpectLang:  expected BCP-47 language tag; empty when undefined.
type CorpusEntry struct {
	Path        string   `yaml:"path"`
	MustInclude []string `yaml:"must_include"`
	MustExclude []string `yaml:"must_exclude"`
	ExpectClass string   `yaml:"expect_class"`
	ExpectLang  string   `yaml:"expect_lang"`
}

// LoadCorpus reads and YAML-decodes a corpus file. Any read or parse error is
// wrapped for caller context.
func LoadCorpus(path string) ([]CorpusEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read corpus: %w", err)
	}
	var entries []CorpusEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse corpus: %w", err)
	}
	return entries, nil
}
