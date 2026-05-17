package engine

import (
	"bytes"
	"encoding/xml"
	"regexp"
	"strings"
)

// TEI-Lite output. Stdlib encoding/xml only — no new deps.
// Best-effort Markdown → TEI conversion. Out of scope: full TEI ODD
// validation, mathML, footnotes, cross-references.

type teiDoc struct {
	XMLName xml.Name  `xml:"TEI"`
	Xmlns   string    `xml:"xmlns,attr"`
	Header  teiHeader `xml:"teiHeader"`
	Text    teiText   `xml:"text"`
}

type teiHeader struct {
	FileDesc    teiFileDesc     `xml:"fileDesc"`
	ProfileDesc *teiProfileDesc `xml:"profileDesc,omitempty"`
}

type teiFileDesc struct {
	TitleStmt       teiTitleStmt       `xml:"titleStmt"`
	PublicationStmt teiPublicationStmt `xml:"publicationStmt"`
	SourceDesc      teiSourceDesc      `xml:"sourceDesc"`
}

type teiTitleStmt struct {
	Title  string `xml:"title"`
	Author string `xml:"author,omitempty"`
}

type teiPublicationStmt struct {
	P string `xml:"p"`
}

type teiSourceDesc struct {
	Bibl teiBibl `xml:"bibl"`
}

type teiBibl struct {
	Ref  teiRef `xml:"ref"`
	Date string `xml:"date,omitempty"`
}

type teiRef struct {
	Target string `xml:"target,attr"`
	Value  string `xml:",chardata"`
}

type teiProfileDesc struct {
	LangUsage teiLangUsage `xml:"langUsage"`
}

type teiLangUsage struct {
	Language teiLanguage `xml:"language"`
}

type teiLanguage struct {
	Ident string `xml:"ident,attr"`
}

type teiText struct {
	Body teiBody `xml:"body"`
}

type teiBody struct {
	Nodes []teiNode `xml:",any"`
}

// teiNode is a polymorphic body element. We choose XMLName per instance so
// encoding/xml emits the right tag. Optional attrs are set only when used.
type teiNode struct {
	XMLName  xml.Name
	Type     string    `xml:"type,attr,omitempty"`
	Lang     string    `xml:"lang,attr,omitempty"`
	Chardata string    `xml:",chardata"`
	Items    []teiNode `xml:",any"`
}

var headingRE = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
var orderedItemRE = regexp.MustCompile(`^\d+\.\s+(.+)$`)
var fenceRE = regexp.MustCompile("(?s)^```([^\\n]*)\\n(.*?)```$")

// ResultToTEIXML wraps a Result into a TEI-Lite XML document.
func ResultToTEIXML(r Result) ([]byte, error) {
	doc := teiDoc{
		Xmlns: "http://www.tei-c.org/ns/1.0",
		Header: teiHeader{
			FileDesc: teiFileDesc{
				TitleStmt: teiTitleStmt{
					Title:  r.Title,
					Author: r.Byline,
				},
				PublicationStmt: teiPublicationStmt{P: "seaportal extraction"},
				SourceDesc: teiSourceDesc{
					Bibl: teiBibl{
						Ref:  teiRef{Target: r.URL, Value: r.URL},
						Date: r.PublishedDate,
					},
				},
			},
		},
		Text: teiText{
			Body: teiBody{Nodes: markdownToTEI(r.Content)},
		},
	}
	if r.Language != "" {
		doc.Header.ProfileDesc = &teiProfileDesc{
			LangUsage: teiLangUsage{Language: teiLanguage{Ident: r.Language}},
		}
	}
	// Empty body → emit one empty <p/>.
	if len(doc.Text.Body.Nodes) == 0 {
		doc.Text.Body.Nodes = []teiNode{{XMLName: xml.Name{Local: "p"}}}
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// markdownToTEI walks Markdown line-by-line, flushing runs of consecutive
// list items or prose paragraphs into single elements. Code fences are
// masked first to keep their content out of the state machine.
func markdownToTEI(md string) []teiNode {
	if strings.TrimSpace(md) == "" {
		return nil
	}

	masked, store := maskCode(md)
	lines := strings.Split(masked, "\n")

	var out []teiNode

	var prose []string
	var ulItems []string
	var olItems []string

	flushProse := func() {
		if len(prose) == 0 {
			return
		}
		text := strings.TrimSpace(strings.Join(prose, " "))
		prose = prose[:0]
		if text == "" {
			return
		}
		// Restore any code-fence/inline-code tokens inside prose.
		text = unmaskCode(text, store)
		out = append(out, teiNode{
			XMLName:  xml.Name{Local: "p"},
			Chardata: text,
		})
	}
	flushUL := func() {
		if len(ulItems) == 0 {
			return
		}
		items := make([]teiNode, 0, len(ulItems))
		for _, it := range ulItems {
			items = append(items, teiNode{
				XMLName:  xml.Name{Local: "item"},
				Chardata: unmaskCode(it, store),
			})
		}
		ulItems = ulItems[:0]
		out = append(out, teiNode{
			XMLName: xml.Name{Local: "list"},
			Type:    "unordered",
			Items:   items,
		})
	}
	flushOL := func() {
		if len(olItems) == 0 {
			return
		}
		items := make([]teiNode, 0, len(olItems))
		for _, it := range olItems {
			items = append(items, teiNode{
				XMLName:  xml.Name{Local: "item"},
				Chardata: unmaskCode(it, store),
			})
		}
		olItems = olItems[:0]
		out = append(out, teiNode{
			XMLName: xml.Name{Local: "list"},
			Type:    "ordered",
			Items:   items,
		})
	}
	flushAll := func() {
		flushProse()
		flushUL()
		flushOL()
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)

		// Blank line ends the current run.
		if trimmed == "" {
			flushAll()
			continue
		}

		// Code-fence sentinel on its own line → emit <code>.
		if strings.HasPrefix(trimmed, "\x00CB") && strings.HasSuffix(trimmed, "\x00") {
			flushAll()
			restored := unmaskCode(trimmed, store)
			lang, body := parseCodeFence(restored)
			node := teiNode{
				XMLName:  xml.Name{Local: "code"},
				Chardata: body,
			}
			if lang != "" {
				node.Lang = lang
			}
			out = append(out, node)
			continue
		}

		// Heading.
		if m := headingRE.FindStringSubmatch(trimmed); m != nil {
			flushAll()
			level := len(m[1])
			out = append(out, teiNode{
				XMLName:  xml.Name{Local: "head"},
				Type:     headingType(level),
				Chardata: unmaskCode(strings.TrimSpace(m[2]), store),
			})
			continue
		}

		// Unordered list item.
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			flushProse()
			flushOL()
			ulItems = append(ulItems, strings.TrimSpace(trimmed[2:]))
			continue
		}

		// Ordered list item.
		if m := orderedItemRE.FindStringSubmatch(trimmed); m != nil {
			flushProse()
			flushUL()
			olItems = append(olItems, strings.TrimSpace(m[1]))
			continue
		}

		// Plain prose. Break any pending list run.
		flushUL()
		flushOL()
		prose = append(prose, trimmed)
	}

	flushAll()
	return out
}

func headingType(level int) string {
	switch level {
	case 1:
		return "h1"
	case 2:
		return "h2"
	case 3:
		return "h3"
	case 4:
		return "h4"
	case 5:
		return "h5"
	default:
		return "h6"
	}
}

// parseCodeFence extracts the info-string language and body from a restored
// fenced block of the form ```lang\nBODY\n```.
func parseCodeFence(s string) (string, string) {
	if m := fenceRE.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1]), strings.TrimRight(m[2], "\n")
	}
	return "", s
}
