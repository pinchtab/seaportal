package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pinchtab/seaportal/internal/corpus"
)

func forEachFixture(corpusPath string, fn func(entry corpus.Entry, html, baseURL string) error) error {
	entries, err := corpus.Load(corpusPath)
	if err != nil {
		return fmt.Errorf("load corpus: %w", err)
	}
	repoRoot := resolveRepoRoot(corpusPath)
	for _, entry := range entries {
		path := entry.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoRoot, path)
		}
		htmlBytes, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read fixture %s: %w", entry.Path, readErr)
		}
		baseURL := "https://corpus.local/" + slugify(entry.Path)
		if err := fn(entry, string(htmlBytes), baseURL); err != nil {
			return err
		}
	}
	return nil
}

func resolveRepoRoot(corpusPath string) string {
	abs, err := filepath.Abs(corpusPath)
	if err != nil {
		return "."
	}
	dir := filepath.Dir(abs)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(abs)
		}
		dir = parent
	}
}

func slugify(p string) string {
	var b strings.Builder
	b.Grow(len(p))
	for _, r := range strings.ToLower(p) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
