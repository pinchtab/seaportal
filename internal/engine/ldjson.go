package engine

import (
	"encoding/json"
	"regexp"
	"strings"
)

var reLDJSON = regexp.MustCompile(`(?is)<script\s+type\s*=\s*["']application/ld\+json["'][^>]*>([\s\S]*?)</script>`)

type LDJSONBlock struct {
	Type        string `json:"type,omitempty"`
	Headline    string `json:"headline,omitempty"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`
	DatePub     string `json:"datePublished,omitempty"`
	Publisher   string `json:"publisher,omitempty"`
	URL         string `json:"url,omitempty"`
	Keywords    string `json:"keywords,omitempty"`
	Language    string `json:"inLanguage,omitempty"`
	Section     string `json:"articleSection,omitempty"`
	Body        string `json:"articleBody,omitempty"`
}

func ExtractLDJSON(html string) []LDJSONBlock {
	matches := reLDJSON.FindAllStringSubmatch(html, 10)
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

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
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

	block.Author = extractAuthor(obj["author"])

	block.Language = extractStringOrNamedObj(obj["inLanguage"])

	block.Section = extractFirstString(obj["articleSection"])

	block.Body = strings.TrimSpace(jsonStr(obj, "articleBody"))

	if pub, ok := obj["publisher"].(map[string]interface{}); ok {
		block.Publisher = jsonStr(pub, "name")
	} else {
		block.Publisher = jsonStr(obj, "publisher")
	}

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

func extractStringOrNamedObj(v interface{}) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case map[string]interface{}:
		return jsonStr(s, "name")
	}
	return ""
}

func extractFirstString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []interface{}:
		for _, item := range s {
			if str, ok := item.(string); ok && str != "" {
				return str
			}
		}
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
