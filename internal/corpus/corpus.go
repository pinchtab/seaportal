package corpus

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Entry struct {
	Path        string   `yaml:"path"`
	MustInclude []string `yaml:"must_include"`
	MustExclude []string `yaml:"must_exclude"`
	ExpectClass string   `yaml:"expect_class"`
	ExpectLang  string   `yaml:"expect_lang"`
}

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
