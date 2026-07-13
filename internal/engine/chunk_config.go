package engine

import (
	"fmt"
	"strconv"
	"strings"
)

type ChunkStrategy int

const (
	ChunkOff ChunkStrategy = iota
	ChunkHeading
	ChunkSentence
	ChunkWindow
)

type ChunkConfig struct {
	Strategy ChunkStrategy
	Size     int
	Overlap  int
}

func (c ChunkConfig) String() string {
	switch c.Strategy {
	case ChunkOff:
		return ""
	case ChunkHeading:
		return "heading"
	case ChunkSentence:
		if c.Size == 0 {
			return "sentence"
		}
		return fmt.Sprintf("sentence:%d", c.Size)
	case ChunkWindow:
		if c.Size == 0 {
			return "window"
		}
		if c.Overlap == 0 {
			return fmt.Sprintf("window:%d", c.Size)
		}
		return fmt.Sprintf("window:%d:%d", c.Size, c.Overlap)
	default:
		return fmt.Sprintf("unknown(%d)", int(c.Strategy))
	}
}

func ParseChunkConfig(s string) (ChunkConfig, error) {
	if s == "" {
		return ChunkConfig{}, nil
	}
	parts := strings.Split(s, ":")
	name := parts[0]
	switch name {
	case "heading":
		if len(parts) > 1 {
			return ChunkConfig{}, fmt.Errorf("invalid chunk config %q: heading takes no parameters", s)
		}
		return ChunkConfig{Strategy: ChunkHeading}, nil

	case "sentence":
		cfg := ChunkConfig{Strategy: ChunkSentence, Size: 512}
		if len(parts) >= 2 && parts[1] != "" {
			n, err := strconv.Atoi(parts[1])
			if err != nil || n <= 0 {
				return ChunkConfig{}, fmt.Errorf("invalid chunk size %q: want positive integer", parts[1])
			}
			cfg.Size = n
		}
		if len(parts) > 2 {
			return ChunkConfig{}, fmt.Errorf("invalid chunk config %q: sentence accepts at most one parameter", s)
		}
		return cfg, nil

	case "window":
		cfg := ChunkConfig{Strategy: ChunkWindow, Size: 2000, Overlap: 200}
		if len(parts) >= 2 && parts[1] != "" {
			n, err := strconv.Atoi(parts[1])
			if err != nil || n <= 0 {
				return ChunkConfig{}, fmt.Errorf("invalid window size %q: want positive integer", parts[1])
			}
			cfg.Size = n
			if len(parts) < 3 {
				cfg.Overlap = 0
			}
		}
		if len(parts) >= 3 && parts[2] != "" {
			o, err := strconv.Atoi(parts[2])
			if err != nil || o < 0 {
				return ChunkConfig{}, fmt.Errorf("invalid window overlap %q: want non-negative integer", parts[2])
			}
			cfg.Overlap = o
		}
		if len(parts) > 3 {
			return ChunkConfig{}, fmt.Errorf("invalid chunk config %q: window accepts at most two parameters", s)
		}
		if cfg.Overlap >= cfg.Size {
			return ChunkConfig{}, fmt.Errorf("invalid chunk config %q: overlap (%d) must be less than size (%d)", s, cfg.Overlap, cfg.Size)
		}
		return cfg, nil

	default:
		return ChunkConfig{}, fmt.Errorf("unknown chunk strategy %q: want heading|sentence|window", name)
	}
}
