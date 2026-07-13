package engine

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

type RankedSection struct {
	Score   float64 `json:"score"`
	Heading string  `json:"heading,omitempty"`
	Text    string  `json:"text"`
	Tokens  int     `json:"tokens"`
}

func RankSections(content, query string, k1, b float64, topN int) []RankedSection {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	if k1 == 0 {
		k1 = 1.5
	}
	if b == 0 {
		b = 0.75
	}

	queryTerms := tokeniseBM25(query)
	if len(queryTerms) == 0 {
		return nil
	}

	raw := chunkByHeading(content)
	if len(raw) == 0 {
		return nil
	}

	type section struct {
		heading string
		text    string
		tokens  []string
		tf      map[string]int
		dl      int
	}

	sections := make([]section, 0, len(raw))
	df := make(map[string]int)
	totalLen := 0

	for _, rc := range raw {
		text := strings.TrimSpace(rc.text)
		if text == "" {
			continue
		}
		toks := tokeniseBM25(text)
		tf := make(map[string]int, len(toks))
		for _, t := range toks {
			tf[t]++
		}
		for term := range tf {
			df[term]++
		}
		heading := strings.TrimSpace(rc.heading)
		sections = append(sections, section{
			heading: heading,
			text:    text,
			tokens:  toks,
			tf:      tf,
			dl:      len(toks),
		})
		totalLen += len(toks)
	}

	N := len(sections)
	if N == 0 {
		return nil
	}
	avgdl := float64(totalLen) / float64(N)
	if avgdl == 0 {
		avgdl = 1
	}

	out := make([]RankedSection, 0, N)
	for _, s := range sections {
		var score float64
		for _, q := range queryTerms {
			tf := float64(s.tf[q])
			if tf == 0 {
				continue
			}
			dfq := float64(df[q])
			idf := math.Log((float64(N)-dfq+0.5)/(dfq+0.5) + 1)
			denom := tf + k1*(1-b+b*float64(s.dl)/avgdl)
			score += idf * (tf * (k1 + 1)) / denom
		}
		out = append(out, RankedSection{
			Score:   score,
			Heading: s.heading,
			Text:    s.text,
			Tokens:  s.dl,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})

	if topN > 0 && topN < len(out) {
		out = out[:topN]
	}
	return out
}

func tokeniseBM25(s string) []string {
	s = strings.ToLower(s)
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}
