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

// Schema is a declarative CSS-selector mapping from output field names to
// extraction specs. Top-level fields capture single values; nested Fields
// (on a FieldSpec) produce arrays of objects (one per matching element);
// Multiple=true produces a flat array of strings.
type Schema struct {
	Fields map[string]FieldSpec `json:"fields" yaml:"fields"`
}

// FieldSpec describes how to extract one field from the DOM.
//
//   - Selector: CSS selector (cascadia syntax). Required.
//   - Attr: when set, the value is the named attribute of the matched element
//     rather than its joined text content.
//   - Multiple: when true (and Fields empty), emit an array of values.
//   - Fields: when non-empty, treat this as a nested-object collector; query
//     QueryAll on Selector and recursively apply Fields scoped to each match,
//     emitting an array of maps.
type FieldSpec struct {
	Selector string               `json:"selector,omitempty" yaml:"selector,omitempty"`
	Attr     string               `json:"attr,omitempty" yaml:"attr,omitempty"`
	Multiple bool                 `json:"multiple,omitempty" yaml:"multiple,omitempty"`
	Fields   map[string]FieldSpec `json:"fields,omitempty" yaml:"fields,omitempty"`
}

const schemaMaxDepth = 5

// LoadSchema reads a schema file from disk and decodes it as YAML or JSON.
// The format is picked from the file extension (.yaml/.yml → YAML, .json →
// JSON). When the extension is ambiguous or unknown we try YAML first and
// fall back to JSON.
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

// ApplySchema parses htmlStr and walks the supplied Schema, returning a
// map[string]interface{} keyed by FieldSpec name. Schema is intended to run
// on RAW html (before preprocess/sanitize) so the caller-supplied selectors
// can target chrome elements like nav/sidebar/footer that the main extraction
// pipeline strips.
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
			// Nested-object array: one map per matched element.
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

// extractValue returns the named attribute when attr is set; otherwise the
// joined, whitespace-collapsed text of n's descendant text nodes.
func extractValue(n *xhtml.Node, attr string) string {
	if attr != "" {
		for _, a := range n.Attr {
			if a.Key == attr {
				return a.Val
			}
		}
		return ""
	}
	return schemaCollapseWhitespace(textOf(n))
}

func textOf(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.TextNode {
			b.WriteString(node.Data)
			b.WriteByte(' ')
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func schemaCollapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
