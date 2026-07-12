package main

// Shared corpus-fixture iteration for the offline lanes (eval, classify,
// tokens, diff). Loading, repo-root resolution, per-fixture reads, and the
// synthetic base URL were previously copy-pasted per lane (with eval
// drifting to a different base-URL scheme); they live here once.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pinchtab/seaportal/internal/corpus"
)

// forEachFixture loads the corpus at corpusPath and invokes fn once per
// entry with the fixture's raw HTML and its synthetic, deterministic base
// URL ("https://corpus.local/<slug>"). Relative fixture paths are resolved
// against the repo root (the nearest go.mod above the corpus file). A load
// error, a fixture read error, or an error returned by fn aborts the walk.
//
// The HTML is handed over as raw bytes-turned-string on purpose: charset
// fixtures must reach each consumer undecoded, so decoding behaviour stays
// part of what the lanes measure.
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

// resolveRepoRoot finds the repo root by walking up from the corpus path
// until a go.mod surfaces. Falls back to the corpus's parent dir on miss.
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

// slugify converts a fixture path into a deterministic URL-safe slug so
// the synthetic baseURL is stable across runs and free of "/" segments
// that would confuse downstream URL parsing.
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
