package engine

import (
	"regexp"
	"strings"
)

type Chunk struct {
	Index   int    `json:"index"`
	Heading string `json:"heading,omitempty"`
	Text    string `json:"text"`
	Tokens  int    `json:"tokens"`
}

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

type rawChunk struct {
	heading string
	text    string
}

var h2tohRE = regexp.MustCompile(`(?m)^(#{2,6}) +(.+)$`)

type headingAnchor struct {
	pos  int
	line string
}

func buildHeadingAnchors(md string) []headingAnchor {
	var anchors []headingAnchor
	for _, m := range h2tohRE.FindAllStringSubmatchIndex(md, -1) {
		end := strings.Index(md[m[0]:], "\n")
		var line string
		if end < 0 {
			line = md[m[0]:]
		} else {
			line = md[m[0] : m[0]+end]
		}
		anchors = append(anchors, headingAnchor{pos: m[0], line: line})
	}
	return anchors
}

func headingAt(anchors []headingAnchor, pos int) string {
	var cur string
	for _, a := range anchors {
		if a.pos <= pos {
			cur = a.line
		} else {
			break
		}
	}
	return cur
}

func chunkByHeading(md string) []rawChunk {
	locs := h2tohRE.FindAllStringSubmatchIndex(md, -1)
	if len(locs) == 0 {
		return []rawChunk{{text: md}}
	}

	var out []rawChunk
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
		headingEnd := strings.Index(section, "\n")
		var headingLine string
		if headingEnd >= 0 {
			headingLine = section[:headingEnd]
		} else {
			headingLine = section
		}
		out = append(out, rawChunk{heading: headingLine, text: section})
	}

	var soft []rawChunk
	for _, c := range out {
		soft = append(soft, softSplitChunk(c, softSplitThresholdChars)...)
	}
	return soft
}

const softSplitThresholdChars = 600

const softSplitMaxSubChunks = 50

var tableRowRE = regexp.MustCompile(`^\|.*\|.*\|`)

var tableSeparatorRE = regexp.MustCompile(`^\|[\s\-:|]+\|\s*$`)

var exclusiveBoldRE = regexp.MustCompile(`^\*\*[^*]+\*\*\s*:?\s*$`)

var softSplitFenceRE = regexp.MustCompile("^(```|~~~)")

func softSplitChunk(c rawChunk, threshold int) []rawChunk {
	if len(c.text) < threshold {
		return []rawChunk{c}
	}
	lines := strings.Split(c.text, "\n")

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
		return []rawChunk{c}
	}

	parentHeading := strings.TrimSpace(c.heading)

	var out []rawChunk
	var buf strings.Builder
	curHeading := c.heading
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

	for i, l := range lines {
		if i == 0 && parentHeading != "" && strings.TrimSpace(l) == parentHeading {
			buf.WriteString(l)
			buf.WriteByte('\n')
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
			flush()
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

	if len(out) == 0 {
		return []rawChunk{c}
	}
	return out
}

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

func boundaryKey(line string) string {
	if tableRowRE.MatchString(line) {
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

func stripMarkdownInline(s string) string {
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

var sentenceSplitRE = regexp.MustCompile(`(?:[.!?]+\s+)|(?:\n\s*\n)`)

func chunkBySentence(md string, sizeTokens int) []rawChunk {
	budget := sizeTokens * charsPerToken
	if budget <= 0 {
		budget = 512 * charsPerToken
	}

	anchors := buildHeadingAnchors(md)

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
			curHeading = headingAt(anchors, sg.start)
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

	anchors := buildHeadingAnchors(md)

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
		out = append(out, rawChunk{heading: headingAt(anchors, start), text: piece})
		if end == n {
			break
		}
	}
	return out
}
