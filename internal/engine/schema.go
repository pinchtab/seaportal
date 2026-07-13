package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andybalholm/cascadia"
	xhtml "golang.org/x/net/html"
	"gopkg.in/yaml.v3"
)

type Schema struct {
	Fields map[string]FieldSpec `json:"fields" yaml:"fields"`
}

type FieldSpec struct {
	Selector string               `json:"selector,omitempty" yaml:"selector,omitempty"`
	Attr     string               `json:"attr,omitempty" yaml:"attr,omitempty"`
	Multiple bool                 `json:"multiple,omitempty" yaml:"multiple,omitempty"`
	Fields   map[string]FieldSpec `json:"fields,omitempty" yaml:"fields,omitempty"`
}

const schemaMaxDepth = 5

func LoadSchema(path string) (Schema, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Schema{}, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	var s Schema
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(raw, &s); err != nil {
			return Schema{}, fmt.Errorf("yaml decode: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(raw, &s); err != nil {
			return Schema{}, fmt.Errorf("json decode: %w", err)
		}
	default:
		if err := yaml.Unmarshal(raw, &s); err != nil {
			if jerr := json.Unmarshal(raw, &s); jerr != nil {
				return Schema{}, fmt.Errorf("yaml/json decode failed: yaml=%v json=%v", err, jerr)
			}
		}
	}
	return s, nil
}

func ApplySchema(htmlStr string, schema Schema) (map[string]interface{}, error) {
	doc, err := xhtml.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil, fmt.Errorf("schema parse: %w", err)
	}
	return applySchemaToNode(doc, schema.Fields, 0)
}

func applySchemaToNode(parent *xhtml.Node, fields map[string]FieldSpec, depth int) (map[string]interface{}, error) {
	if depth >= schemaMaxDepth {
		return nil, fmt.Errorf("schema nesting exceeded depth %d", schemaMaxDepth)
	}
	out := make(map[string]interface{}, len(fields))
	for name, spec := range fields {
		sel, err := cascadia.Compile(spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("field %q: invalid selector %q: %w", name, spec.Selector, err)
		}
		matches := cascadia.QueryAll(parent, sel)

		switch {
		case len(spec.Fields) > 0:
			items := make([]map[string]interface{}, 0, len(matches))
			for _, m := range matches {
				child, cerr := applySchemaToNode(m, spec.Fields, depth+1)
				if cerr != nil {
					return nil, cerr
				}
				items = append(items, child)
			}
			out[name] = items
		case spec.Multiple:
			vals := make([]string, 0, len(matches))
			for _, m := range matches {
				vals = append(vals, extractValue(m, spec.Attr))
			}
			out[name] = vals
		default:
			if len(matches) == 0 {
				out[name] = ""
			} else {
				out[name] = extractValue(matches[0], spec.Attr)
			}
		}
	}
	return out, nil
}

func extractValue(n *xhtml.Node, attr string) string {
	if attr != "" {
		return getAttr(n, attr)
	}
	return collapseUnicodeWhitespace(nodeTextOpts(n, textOptions{spaceJoin: true}))
}
