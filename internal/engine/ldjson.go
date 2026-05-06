package engine

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ldjson.go — Extract structured data from <script type="application/ld+json"> blocks.
// Rich metadata source for news sites (NYT, BBC), academic pages (arXiv), etc.

var reLDJSON = regexp.MustCompile(`(?is)<script\s+type\s*=\s*["']application/ld\+json["'][^>]*>([\s\S]*?)</script>`)

// LDJSONBlock represents a single LD+JSON structured data block.
type LDJSONBlock struct {
	Type        string `json:"type,omitempty"`          // @type field
	Headline    string `json:"headline,omitempty"`      // Article headline
	Description string `json:"description,omitempty"`   // Article description/abstract
	Author      string `json:"author,omitempty"`        // Author name(s)
	DatePub     string `json:"datePublished,omitempty"` // Publication date
	Publisher   string `json:"publisher,omitempty"`     // Publisher name
	URL         string `json:"url,omitempty"`           // Canonical URL
	Keywords    string `json:"keywords,omitempty"`      // Keywords/tags
}

// ExtractLDJSON extracts and parses all LD+JSON blocks from HTML.
func ExtractLDJSON(html string) []LDJSONBlock {
	matches := reLDJSON.FindAllStringSubmatch(html, 10) // max 10 blocks
	if len(matches) == 0 {
		return nil
	}

	var blocks []LDJSONBlock
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		raw := strings.TrimSpace(m[1])
		if raw == "" {
			continue
		}

		block := parseLDJSONBlock(raw)
		if block.Type != "" || block.Headline != "" || block.Description != "" {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// LDJSONToMarkdown converts LD+JSON blocks into supplementary markdown content.
// Returns empty string if no useful content found.
func LDJSONToMarkdown(blocks []LDJSONBlock) string {
	if len(blocks) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, b := range blocks {
		if b.Headline == "" && b.Description == "" {
			continue
		}
		if b.Headline != "" {
			sb.WriteString("## ")
			sb.WriteString(b.Headline)
			sb.WriteString("\n\n")
		}
		if b.Author != "" || b.DatePub != "" || b.Publisher != "" {
			var meta []string
			if b.Author != "" {
				meta = append(meta, "By "+b.Author)
			}
			if b.DatePub != "" {
				meta = append(meta, b.DatePub)
			}
			if b.Publisher != "" {
				meta = append(meta, b.Publisher)
			}
			sb.WriteString(strings.Join(meta, " · "))
			sb.WriteString("\n\n")
		}
		if b.Description != "" {
			sb.WriteString(b.Description)
			sb.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

func parseLDJSONBlock(raw string) LDJSONBlock {
	var block LDJSONBlock

	// Try parsing as object first.
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		// Try as array (some sites wrap in array). Use []interface{} so mixed
		// element types (object | string | nested array) parse cleanly.
		var arr []interface{}
		if err2 := json.Unmarshal([]byte(raw), &arr); err2 != nil || len(arr) == 0 {
			return block
		}
		var first LDJSONBlock
		for i, item := range arr {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			b := extractFromObj(m)
			if i == 0 {
				first = b
			}
			if b.Headline != "" || b.Description != "" {
				return b
			}
		}
		return first
	}

	// Check for @graph pattern (used by many news sites).
	if graph, ok := obj["@graph"]; ok {
		if items, ok := graph.([]interface{}); ok {
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					b := extractFromObj(m)
					if b.Headline != "" || b.Description != "" {
						return b
					}
				}
			}
		}
	}

	return extractFromObj(obj)
}

func extractFromObj(obj map[string]interface{}) LDJSONBlock {
	block := LDJSONBlock{
		Type:        jsonStr(obj, "@type"),
		Headline:    jsonStr(obj, "headline"),
		Description: jsonStr(obj, "description"),
		DatePub:     jsonStr(obj, "datePublished"),
		URL:         jsonStr(obj, "url"),
		Keywords:    jsonStr(obj, "keywords"),
	}

	// Author can be string, object, or array.
	block.Author = extractAuthor(obj["author"])

	// Publisher can be object with name.
	if pub, ok := obj["publisher"].(map[string]interface{}); ok {
		block.Publisher = jsonStr(pub, "name")
	} else {
		block.Publisher = jsonStr(obj, "publisher")
	}

	// Fallback: name field if no headline.
	if block.Headline == "" {
		block.Headline = jsonStr(obj, "name")
	}

	return block
}

func extractAuthor(v interface{}) string {
	if v == nil {
		return ""
	}
	switch a := v.(type) {
	case string:
		return a
	case map[string]interface{}:
		return jsonStr(a, "name")
	case []interface{}:
		var names []string
		for _, item := range a {
			switch ai := item.(type) {
			case string:
				names = append(names, ai)
			case map[string]interface{}:
				if n := jsonStr(ai, "name"); n != "" {
					names = append(names, n)
				}
			}
		}
		return strings.Join(names, ", ")
	}
	return ""
}

func jsonStr(obj map[string]interface{}, key string) string {
	v, ok := obj[key]
	if !ok {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []interface{}:
		// keywords can be an array of strings.
		var parts []string
		for _, item := range s {
			if str, ok := item.(string); ok {
				parts = append(parts, str)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}
