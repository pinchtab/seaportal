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

type SplitConfig struct {
	Dir      string
	MaxBytes int
	BaseName string
	Format   string
}

type SplitFile struct {
	Path  string `json:"path"`
	Index int    `json:"index"`
	Of    int    `json:"of"`
	Bytes int    `json:"bytes"`
}

const defaultSplitBytes = 32 * 1024

var nonAlnumRE = regexp.MustCompile(`[^a-z0-9]+`)

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
			flush()
			fmt.Fprintf(os.Stderr, "split: oversized chunk %d: %d bytes (cap %d)\n", i, len(u), maxBytes)
			shards = append(shards, u)
			continue
		}
		add := len(u)
		if cur.Len() > 0 {
			add += 2
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
