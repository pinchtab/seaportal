// Package engine output file splitting.
//
// SplitResultToFiles shards a Result.Content (or its existing Chunks) into
// multiple on-disk files under a target directory, each capped at an
// approximate byte budget. Useful for piping a large extraction into LLMs
// with fixed context budgets or sharding archival pipelines.
//
// Behaviour summary:
//   - When r.Chunks is non-empty, the existing chunks are the unit of
//     packing (they're never split further; oversized ones emit a stderr
//     warning and land in their own file).
//   - Otherwise r.Content is split on paragraph boundaries (\n\n) into
//     pseudo-chunks and packed the same way.
//   - Files are written atomically (.tmp + rename) with mode 0644.
//   - Filename pattern: <base>-NNN.<ext> where base is derived from a
//     URL slug (host + path, non-alphanumeric → '-', collapsed, truncated
//     to 60 chars, fallback "seaportal").
//   - Format "md" writes raw markdown text; "json" wraps each shard in
//     {"index":N,"of":K,"url":...,"title":...,"text":...}.

package engine

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SplitConfig controls SplitResultToFiles.
type SplitConfig struct {
	Dir      string
	MaxBytes int    // soft cap per file; 0 = use default (32 KB)
	BaseName string // optional; defaults to URL-slug-derived name
	Format   string // "md" | "json" (default "md")
}

// SplitFile is one entry in the manifest returned by SplitResultToFiles.
type SplitFile struct {
	Path  string `json:"path"`
	Index int    `json:"index"`
	Of    int    `json:"of"`
	Bytes int    `json:"bytes"`
}

const defaultSplitBytes = 32 * 1024

var nonAlnumRE = regexp.MustCompile(`[^a-z0-9]+`)

// slugFromURL derives a filename-safe base from a URL: lowercased host + path,
// non-alphanumeric runs collapsed to '-', trimmed, truncated to 60 chars.
// Returns "seaportal" when the URL is empty or yields no usable characters.
func slugFromURL(raw string) string {
	if raw == "" {
		return "seaportal"
	}
	u, err := url.Parse(raw)
	var s string
	if err != nil || u.Host == "" {
		s = raw
	} else {
		s = strings.ToLower(u.Host) + u.Path
	}
	s = strings.ToLower(s)
	s = nonAlnumRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "seaportal"
	}
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	if s == "" {
		return "seaportal"
	}
	return s
}

// SplitResultToFiles writes r's content into one or more files under cfg.Dir.
// Prefers r.Chunks when present; otherwise paragraph-splits r.Content. Returns
// the manifest of written files with absolute paths. Empty Content + no chunks
// returns (nil, nil) and writes nothing.
func SplitResultToFiles(r Result, cfg SplitConfig) ([]SplitFile, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("split: Dir is required")
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultSplitBytes
	}
	format := cfg.Format
	if format == "" {
		format = "md"
	}
	if format != "md" && format != "json" {
		return nil, fmt.Errorf("split: unsupported format %q (want md|json)", format)
	}
	base := cfg.BaseName
	if base == "" {
		base = slugFromURL(r.URL)
	}

	// Build unit list (chunk texts).
	var units []string
	if len(r.Chunks) > 0 {
		units = make([]string, 0, len(r.Chunks))
		for _, c := range r.Chunks {
			t := strings.TrimSpace(c.Text)
			if t != "" {
				units = append(units, t)
			}
		}
	} else if strings.TrimSpace(r.Content) != "" {
		for _, p := range strings.Split(r.Content, "\n\n") {
			t := strings.TrimSpace(p)
			if t != "" {
				units = append(units, t)
			}
		}
	}
	if len(units) == 0 {
		return nil, nil
	}

	// Pack units into shards.
	var shards []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		shards = append(shards, cur.String())
		cur.Reset()
	}
	for i, u := range units {
		if len(u) > maxBytes {
			// Flush current shard, then emit oversized unit alone.
			flush()
			fmt.Fprintf(os.Stderr, "split: oversized chunk %d: %d bytes (cap %d)\n", i, len(u), maxBytes)
			shards = append(shards, u)
			continue
		}
		// If adding would exceed cap (and we have content), flush first.
		add := len(u)
		if cur.Len() > 0 {
			add += 2 // "\n\n"
		}
		if cur.Len() > 0 && cur.Len()+add > maxBytes {
			flush()
		}
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(u)
	}
	flush()

	if len(shards) == 0 {
		return nil, nil
	}

	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return nil, fmt.Errorf("split: mkdir %s: %w", cfg.Dir, err)
	}
	absDir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		absDir = cfg.Dir
	}

	ext := "md"
	if format == "json" {
		ext = "json"
	}

	manifest := make([]SplitFile, 0, len(shards))
	total := len(shards)
	for i, text := range shards {
		idx := i + 1
		name := fmt.Sprintf("%s-%03d.%s", base, idx, ext)
		full := filepath.Join(absDir, name)

		var payload []byte
		if format == "json" {
			obj := map[string]interface{}{
				"index": idx,
				"of":    total,
				"url":   r.URL,
				"title": r.Title,
				"text":  text,
			}
			b, err := json.Marshal(obj)
			if err != nil {
				return nil, fmt.Errorf("split: json marshal shard %d: %w", idx, err)
			}
			payload = b
		} else {
			payload = []byte(text)
		}

		if err := atomicWrite(full, payload); err != nil {
			return nil, fmt.Errorf("split: write %s: %w", full, err)
		}

		manifest = append(manifest, SplitFile{
			Path:  full,
			Index: idx,
			Of:    total,
			Bytes: len(payload),
		})
	}
	return manifest, nil
}
