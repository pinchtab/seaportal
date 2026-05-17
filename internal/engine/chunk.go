package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ChunkStrategy selects a chunking algorithm. Default is off.
type ChunkStrategy int

const (
	// ChunkOff disables chunking (default — Result.Chunks stays nil).
	ChunkOff ChunkStrategy = iota
	// ChunkHeading splits at H2-H6 boundaries, preserving the heading.
	ChunkHeading
	// ChunkSentence groups sentences until a ~Size-token threshold.
	ChunkSentence
	// ChunkWindow slides a Size-char window with Overlap chars of backstep.
	ChunkWindow
)

// ChunkConfig controls how Markdown is split into Chunks.
type ChunkConfig struct {
	Strategy ChunkStrategy
	// Size meaning depends on Strategy:
	//   sentence: target tokens per group
	//   window:   chars per window
	Size int
	// Overlap is only used by window strategy (chars of overlap).
	Overlap int
}

// Chunk is a single piece of a chunked Markdown body.
type Chunk struct {
	Index   int    `json:"index"`
	Heading string `json:"heading,omitempty"`
	Text    string `json:"text"`
	Tokens  int    `json:"tokens"`
}

// String renders the canonical CLI form of the config (or "" for off).
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

// ParseChunkConfig parses the colon-form CLI argument.
//
//	""                          -> {ChunkOff, 0, 0}
//	"heading"                   -> {ChunkHeading, 0, 0}
//	"sentence" / "sentence:N"   -> {ChunkSentence, N|512, 0}
//	"window" / "window:N[:O]"   -> {ChunkWindow, N|2000, O|200}
//
// Validation:
//   - overlap must be < size (window strategy)
//   - unknown names error out
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
			// When the caller specifies size but no overlap, default overlap to 0
			// rather than 200 — keeps "window:N" unambiguous.
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

// ChunkMarkdown applies cfg to md and returns the resulting Chunks. Returns
// nil when chunking is off, content is empty, or content is shorter than
// 100 chars (not worth chunking).
//
// Code regions are masked before splitting and unmasked per emitted chunk so
// fenced code blocks are never split by heading/sentence boundaries inside.
func ChunkMarkdown(md string, cfg ChunkConfig) []Chunk {
	if cfg.Strategy == ChunkOff {
		return nil
	}
	if md == "" || len(md) < 100 {
		return nil
	}

	masked, store := maskCode(md)

	var raw []rawChunk
	switch cfg.Strategy {
	case ChunkHeading:
		raw = chunkByHeading(masked)
	case ChunkSentence:
		size := cfg.Size
		if size <= 0 {
			size = 512
		}
		raw = chunkBySentence(masked, size)
	case ChunkWindow:
		size := cfg.Size
		if size <= 0 {
			size = 2000
		}
		raw = chunkByWindow(masked, size, cfg.Overlap)
	default:
		return nil
	}

	out := make([]Chunk, 0, len(raw))
	for _, rc := range raw {
		text := strings.TrimSpace(unmaskCode(rc.text, store))
		if text == "" {
			continue
		}
		heading := strings.TrimSpace(unmaskCode(rc.heading, store))
		out = append(out, Chunk{
			Index:   len(out),
			Heading: heading,
			Text:    text,
			Tokens:  len(text) / charsPerToken,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// rawChunk is the pre-unmask intermediate produced by each strategy.
type rawChunk struct {
	heading string
	text    string
}

// h2tohRE matches an H2..H6 line start (multiline). H1 is excluded
// deliberately: extracted bodies often retain an article H1 title that
// shouldn't be treated as a section boundary.
var h2tohRE = regexp.MustCompile(`(?m)^(#{2,6}) +(.+)$`)

// chunkByHeading splits the (masked) markdown on H2..H6 boundaries. The
// preamble before the first heading becomes the first chunk with an empty
// heading. Each subsequent chunk includes its heading line plus the body
// up to (but not including) the next heading.
func chunkByHeading(md string) []rawChunk {
	locs := h2tohRE.FindAllStringSubmatchIndex(md, -1)
	if len(locs) == 0 {
		return []rawChunk{{text: md}}
	}

	var out []rawChunk
	// Preamble.
	if locs[0][0] > 0 {
		pre := md[:locs[0][0]]
		if strings.TrimSpace(pre) != "" {
			out = append(out, rawChunk{text: pre})
		}
	}

	for i, m := range locs {
		start := m[0]
		end := len(md)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		section := md[start:end]
		// The heading line itself: from start to first newline.
		headingEnd := strings.Index(section, "\n")
		var headingLine string
		if headingEnd >= 0 {
			headingLine = section[:headingEnd]
		} else {
			headingLine = section
		}
		out = append(out, rawChunk{heading: headingLine, text: section})
	}

	// Second pass: soft-split oversized chunks on table-row / exclusive-bold
	// boundaries. Reference pages (MDN HTTP methods, glossaries, API refs)
	// frequently pack many entries under a single H2; without this, BM25 can
	// never surface a per-entry chunk.
	var soft []rawChunk
	for _, c := range out {
		soft = append(soft, softSplitChunk(c, softSplitThresholdChars)...)
	}
	return soft
}

// softSplitThresholdChars is the minimum chunk length (in chars, ~150 tokens
// at charsPerToken=4) at which the heading-pass output becomes eligible for
// the table-row / bold-paragraph soft-split second pass. Smaller chunks are
// left alone so that short meta-sections (e.g. "## Specifications") are not
// over-fragmented. Grep-able knob.
const softSplitThresholdChars = 600

// softSplitMaxSubChunks caps the number of sub-chunks produced from a single
// parent chunk. Tables with hundreds of rows would otherwise explode chunk
// count and bloat BM25 cost; if the cap is exceeded we fall back to returning
// the parent un-split.
const softSplitMaxSubChunks = 50

// tableRowRE matches a Markdown table row: a line containing at least two
// pipes. Used as a boundary marker for soft-splitting reference pages that
// encode entries (HTTP methods, glossary terms, API columns) as table rows.
var tableRowRE = regexp.MustCompile(`^\|.*\|.*\|`)

// tableSeparatorRE matches a Markdown table separator row (only pipes,
// dashes, colons, spaces). These are structural and must NOT start a new
// sub-chunk on their own.
var tableSeparatorRE = regexp.MustCompile(`^\|[\s\-:|]+\|\s*$`)

// exclusiveBoldRE matches a line whose entire visible content is a single
// bold span (e.g. "**Note**" or "**Term:**"). Used as a boundary marker for
// definition-list-style entries that survived readability as bold-prefixed
// paragraphs. Embedded bold inside prose is intentionally NOT matched.
var exclusiveBoldRE = regexp.MustCompile(`^\*\*[^*]+\*\*\s*:?\s*$`)

// softSplitFenceRE matches the start/end of a fenced code block (``` or ~~~).
var softSplitFenceRE = regexp.MustCompile("^(```|~~~)")

// softSplitChunk takes a heading-bounded chunk and, if it exceeds threshold
// chars AND contains table-row or exclusive-bold boundaries, fragments it so
// each boundary becomes its own sub-chunk. Sub-chunk headings are
// "<parent heading> · <boundary key>" so downstream consumers can still trace
// provenance. Lines inside fenced code blocks are NEVER treated as
// boundaries. If the soft-split would produce more than softSplitMaxSubChunks
// pieces, we return the parent un-split (runaway-table guard).
func softSplitChunk(c rawChunk, threshold int) []rawChunk {
	if len(c.text) < threshold {
		return []rawChunk{c}
	}
	lines := strings.Split(c.text, "\n")

	// Pre-scan: count boundaries (outside code fences). If none, no work to do.
	inFence := false
	boundaryCount := 0
	for _, l := range lines {
		if softSplitFenceRE.MatchString(strings.TrimSpace(l)) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if isSoftSplitBoundary(l) {
			boundaryCount++
		}
	}
	if boundaryCount == 0 {
		return []rawChunk{c}
	}
	if boundaryCount > softSplitMaxSubChunks {
		// Runaway: fall back to the parent. Documented above.
		return []rawChunk{c}
	}

	// Detect whether the chunk begins with a Markdown heading line (true for
	// every chunk except the document preamble). When present, that heading
	// is the parent and stays atop the preamble sub-chunk; sub-chunks get
	// "<parent> · <boundary>" headings.
	parentHeading := strings.TrimSpace(c.heading)

	var out []rawChunk
	var buf strings.Builder
	curHeading := c.heading // preamble inherits parent heading verbatim
	inFence = false

	flush := func() {
		t := strings.TrimRight(buf.String(), "\n")
		if strings.TrimSpace(t) == "" {
			buf.Reset()
			return
		}
		out = append(out, rawChunk{heading: curHeading, text: t})
		buf.Reset()
	}

	headingLineConsumed := false
	for i, l := range lines {
		// Keep the chunk's own heading line attached to whatever sub-chunk
		// comes first (preamble or first boundary).
		if i == 0 && parentHeading != "" && strings.TrimSpace(l) == parentHeading {
			buf.WriteString(l)
			buf.WriteByte('\n')
			headingLineConsumed = true
			continue
		}

		trimmed := strings.TrimSpace(l)
		if softSplitFenceRE.MatchString(trimmed) {
			inFence = !inFence
			buf.WriteString(l)
			buf.WriteByte('\n')
			continue
		}
		if inFence {
			buf.WriteString(l)
			buf.WriteByte('\n')
			continue
		}

		if isSoftSplitBoundary(l) {
			// Flush previous accumulation as its own sub-chunk.
			flush()
			// New sub-chunk: heading = parent · boundaryKey.
			key := boundaryKey(l)
			if parentHeading != "" && key != "" {
				curHeading = parentHeading + " · " + key
			} else if key != "" {
				curHeading = key
			} else {
				curHeading = parentHeading
			}
			buf.WriteString(l)
			buf.WriteByte('\n')
			continue
		}

		buf.WriteString(l)
		buf.WriteByte('\n')
	}
	flush()

	// Defensive: if we somehow produced nothing (shouldn't happen given the
	// boundaryCount>0 path), return the original.
	if len(out) == 0 {
		return []rawChunk{c}
	}
	_ = headingLineConsumed
	return out
}

// isSoftSplitBoundary reports whether a line should start a new soft-split
// sub-chunk. Table separator rows are intentionally excluded (structural).
func isSoftSplitBoundary(line string) bool {
	if tableSeparatorRE.MatchString(line) {
		return false
	}
	if tableRowRE.MatchString(line) {
		return true
	}
	if exclusiveBoldRE.MatchString(line) {
		return true
	}
	return false
}

// boundaryKey extracts a short, human-readable label from a boundary line:
// the first table cell (stripped of markdown link/code syntax) or the bold
// text. Used to build child-chunk headings of the form "<parent> · <key>".
func boundaryKey(line string) string {
	if tableRowRE.MatchString(line) {
		// First cell: between first and second pipe.
		s := strings.TrimPrefix(line, "|")
		end := strings.Index(s, "|")
		if end < 0 {
			return ""
		}
		cell := strings.TrimSpace(s[:end])
		return stripMarkdownInline(cell)
	}
	if exclusiveBoldRE.MatchString(line) {
		s := strings.TrimSpace(line)
		s = strings.TrimSuffix(s, ":")
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "**")
		s = strings.TrimSuffix(s, "**")
		return strings.TrimSpace(s)
	}
	return ""
}

// stripMarkdownInline removes link syntax `[text](url)` and code backticks
// from a short label so child-chunk headings stay readable.
func stripMarkdownInline(s string) string {
	// [text](url) -> text
	for {
		i := strings.Index(s, "[")
		if i < 0 {
			break
		}
		j := strings.Index(s[i:], "](")
		if j < 0 {
			break
		}
		k := strings.Index(s[i+j:], ")")
		if k < 0 {
			break
		}
		s = s[:i] + s[i+1:i+j] + s[i+j+k+1:]
	}
	s = strings.ReplaceAll(s, "`", "")
	return strings.TrimSpace(s)
}

// sentenceSplitRE splits on sentence-final punctuation followed by whitespace,
// or on a paragraph break.
var sentenceSplitRE = regexp.MustCompile(`(?:[.!?]+\s+)|(?:\n\s*\n)`)

// chunkBySentence groups sentences (and paragraph breaks) until accumulated
// chars >= size*charsPerToken. The most-recent H2..H6 line above each emission
// is carried into Heading.
func chunkBySentence(md string, sizeTokens int) []rawChunk {
	budget := sizeTokens * charsPerToken
	if budget <= 0 {
		budget = 512 * charsPerToken
	}

	// Pre-compute heading anchors: positions of each heading-line start and
	// the heading text itself.
	type anchor struct {
		pos  int
		line string
	}
	var anchors []anchor
	for _, m := range h2tohRE.FindAllStringSubmatchIndex(md, -1) {
		end := strings.Index(md[m[0]:], "\n")
		var line string
		if end < 0 {
			line = md[m[0]:]
		} else {
			line = md[m[0] : m[0]+end]
		}
		anchors = append(anchors, anchor{pos: m[0], line: line})
	}
	headingFor := func(at int) string {
		var cur string
		for _, a := range anchors {
			if a.pos <= at {
				cur = a.line
			} else {
				break
			}
		}
		return cur
	}

	// Sentence-level segmentation.
	splits := sentenceSplitRE.FindAllStringIndex(md, -1)
	type seg struct {
		start, end int
	}
	var segs []seg
	cursor := 0
	for _, s := range splits {
		if s[1] > cursor {
			segs = append(segs, seg{start: cursor, end: s[1]})
			cursor = s[1]
		}
	}
	if cursor < len(md) {
		segs = append(segs, seg{start: cursor, end: len(md)})
	}

	var out []rawChunk
	var buf strings.Builder
	bufStart := -1
	curHeading := ""
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		out = append(out, rawChunk{heading: curHeading, text: buf.String()})
		buf.Reset()
		bufStart = -1
	}

	for _, sg := range segs {
		piece := md[sg.start:sg.end]
		if bufStart == -1 {
			bufStart = sg.start
			curHeading = headingFor(sg.start)
		}
		buf.WriteString(piece)
		if buf.Len() >= budget {
			flush()
		}
	}
	flush()

	if len(out) == 0 {
		return []rawChunk{{text: md}}
	}
	return out
}

// chunkByWindow slides a sizeChars window over the body, stepping by
// (size - overlap). End-of-window snaps back to the nearest space within
// size/4 chars to avoid mid-word breaks. Heading inherited from the
// most-recent H2..H6 above each window start.
func chunkByWindow(md string, sizeChars, overlapChars int) []rawChunk {
	if sizeChars <= 0 {
		sizeChars = 2000
	}
	if overlapChars < 0 {
		overlapChars = 0
	}
	if overlapChars >= sizeChars {
		overlapChars = sizeChars / 2
	}
	step := sizeChars - overlapChars
	if step <= 0 {
		step = sizeChars
	}

	// Heading anchors (reuse heading-by-position lookup).
	type anchor struct {
		pos  int
		line string
	}
	var anchors []anchor
	for _, m := range h2tohRE.FindAllStringSubmatchIndex(md, -1) {
		end := strings.Index(md[m[0]:], "\n")
		var line string
		if end < 0 {
			line = md[m[0]:]
		} else {
			line = md[m[0] : m[0]+end]
		}
		anchors = append(anchors, anchor{pos: m[0], line: line})
	}
	headingFor := func(at int) string {
		var cur string
		for _, a := range anchors {
			if a.pos <= at {
				cur = a.line
			} else {
				break
			}
		}
		return cur
	}

	wordSlack := sizeChars / 4
	if wordSlack < 1 {
		wordSlack = 1
	}

	var out []rawChunk
	n := len(md)
	for start := 0; start < n; start += step {
		end := start + sizeChars
		if end >= n {
			end = n
		} else {
			// Snap end back to the nearest space within wordSlack.
			snap := end
			for i := 0; i < wordSlack && snap > start; i++ {
				if snap-1 < n && (md[snap-1] == ' ' || md[snap-1] == '\n') {
					break
				}
				snap--
			}
			if snap > start {
				end = snap
			}
		}
		piece := md[start:end]
		out = append(out, rawChunk{heading: headingFor(start), text: piece})
		if end == n {
			break
		}
	}
	return out
}
