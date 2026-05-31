package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplySchema_SingleValueField(t *testing.T) {
	html := `<html><body><h1>  Hello   World  </h1></body></html>`
	s := Schema{Fields: map[string]FieldSpec{"title": {Selector: "h1"}}}
	out, err := ApplySchema(html, s)
	if err != nil {
		t.Fatal(err)
	}
	if got := out["title"]; got != "Hello World" {
		t.Fatalf("got %q", got)
	}
}

func TestApplySchema_MultipleValues(t *testing.T) {
	html := `<html><body><span class="tag">a</span><span class="tag">b</span><span class="tag">c</span></body></html>`
	s := Schema{Fields: map[string]FieldSpec{"tags": {Selector: ".tag", Multiple: true}}}
	out, err := ApplySchema(html, s)
	if err != nil {
		t.Fatal(err)
	}
	tags, ok := out["tags"].([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", out["tags"])
	}
	if len(tags) != 3 || tags[0] != "a" || tags[1] != "b" || tags[2] != "c" {
		t.Fatalf("got %v", tags)
	}
}

func TestApplySchema_NestedObjects(t *testing.T) {
	html := `<html><body>
		<div class="product"><span class="name">Widget</span><span class="price" data-price="9.99">$9.99</span></div>
		<div class="product"><span class="name">Gadget</span><span class="price" data-price="19.99">$19.99</span></div>
	</body></html>`
	s := Schema{Fields: map[string]FieldSpec{
		"products": {
			Selector: ".product",
			Fields: map[string]FieldSpec{
				"name":  {Selector: ".name"},
				"price": {Selector: ".price", Attr: "data-price"},
			},
		},
	}}
	out, err := ApplySchema(html, s)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := out["products"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected []map, got %T", out["products"])
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0]["name"] != "Widget" || items[0]["price"] != "9.99" {
		t.Fatalf("item 0: %v", items[0])
	}
	if items[1]["name"] != "Gadget" || items[1]["price"] != "19.99" {
		t.Fatalf("item 1: %v", items[1])
	}
}

func TestApplySchema_AttrExtraction(t *testing.T) {
	html := `<html><body><a href="https://example.com/x">link</a></body></html>`
	s := Schema{Fields: map[string]FieldSpec{"link": {Selector: "a", Attr: "href"}}}
	out, err := ApplySchema(html, s)
	if err != nil {
		t.Fatal(err)
	}
	if got := out["link"]; got != "https://example.com/x" {
		t.Fatalf("got %v", got)
	}
}

func TestApplySchema_MissingSelectorEmpty(t *testing.T) {
	html := `<html><body><p>nothing here</p></body></html>`
	s := Schema{Fields: map[string]FieldSpec{
		"single":  {Selector: ".missing"},
		"many":    {Selector: ".missing", Multiple: true},
		"objects": {Selector: ".missing", Fields: map[string]FieldSpec{"x": {Selector: "span"}}},
	}}
	out, err := ApplySchema(html, s)
	if err != nil {
		t.Fatal(err)
	}
	if out["single"] != "" {
		t.Fatalf("single: %v", out["single"])
	}
	if many, ok := out["many"].([]string); !ok || len(many) != 0 {
		t.Fatalf("many: %v", out["many"])
	}
	if objs, ok := out["objects"].([]map[string]interface{}); !ok || len(objs) != 0 {
		t.Fatalf("objects: %v", out["objects"])
	}
}

func TestApplySchema_InvalidSelectorErrors(t *testing.T) {
	s := Schema{Fields: map[string]FieldSpec{"bad": {Selector: "::::!!!"}}}
	if _, err := ApplySchema(`<html></html>`, s); err == nil {
		t.Fatal("expected error for invalid selector")
	}
}

func TestApplySchema_NestedScope(t *testing.T) {
	// .name outside .box should NOT leak into the nested result.
	html := `<html><body>
		<span class="name">leaked</span>
		<div class="box"><span class="name">inside</span></div>
	</body></html>`
	s := Schema{Fields: map[string]FieldSpec{
		"boxes": {
			Selector: ".box",
			Fields: map[string]FieldSpec{
				"name": {Selector: ".name"},
			},
		},
	}}
	out, err := ApplySchema(html, s)
	if err != nil {
		t.Fatal(err)
	}
	items := out["boxes"].([]map[string]interface{})
	if len(items) != 1 || items[0]["name"] != "inside" {
		t.Fatalf("expected scope to 'inside', got %v", items)
	}
}

func TestApplySchema_DepthBound(t *testing.T) {
	// Build a schema nested 6 levels deep (over the depth-5 cap).
	leaf := FieldSpec{Selector: "x"}
	cur := leaf
	for i := 0; i < 6; i++ {
		cur = FieldSpec{Selector: "div", Fields: map[string]FieldSpec{"child": cur}}
	}
	s := Schema{Fields: map[string]FieldSpec{"root": cur}}
	html := `<html><body><div><div><div><div><div><div><x>y</x></div></div></div></div></div></div></body></html>`
	if _, err := ApplySchema(html, s); err == nil {
		t.Fatal("expected depth-bound error")
	}
}

func TestLoadSchema_YAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.yaml")
	if err := os.WriteFile(p, []byte("fields:\n  title:\n    selector: h1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSchema(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Fields["title"].Selector != "h1" {
		t.Fatalf("got %+v", s)
	}
}

func TestLoadSchema_JSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.json")
	body := `{"fields":{"title":{"selector":"h1"}}}`
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSchema(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Fields["title"].Selector != "h1" {
		t.Fatalf("got %+v", s)
	}
}

func TestLoadSchema_FileNotFound(t *testing.T) {
	if _, err := LoadSchema("/no/such/path.yaml"); err == nil {
		t.Fatal("expected error")
	}
}

func TestExtract_SchemaPath(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "s.yaml")
	schemaBody := `fields:
  title:
    selector: h1
  tags:
    selector: .tag
    multiple: true
  products:
    selector: .product
    fields:
      name:
        selector: .name
      price:
        selector: .price
        attr: data-price
`
	if err := os.WriteFile(schemaPath, []byte(schemaBody), 0644); err != nil {
		t.Fatal(err)
	}

	html := `<!doctype html><html><head><title>T</title></head><body>
		<h1>Catalog</h1>
		<span class="tag">red</span><span class="tag">blue</span>
		<div class="product"><span class="name">Widget</span><span class="price" data-price="9.99">$9.99</span></div>
		<div class="product"><span class="name">Gadget</span><span class="price" data-price="19.99">$19.99</span></div>
		<p>Some prose paragraph so readability has something to chew on. ` + strings.Repeat("Lorem ipsum dolor sit amet. ", 20) + `</p>
	</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	res := FromURLWithOptions(srv.URL, Options{SchemaPath: schemaPath})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if res.Schema == nil {
		t.Fatalf("expected schema populated; warnings=%v", res.Warnings)
	}
	if res.Schema["title"] != "Catalog" {
		t.Fatalf("title: %v", res.Schema["title"])
	}
	tags, ok := res.Schema["tags"].([]string)
	if !ok || len(tags) != 2 || tags[0] != "red" || tags[1] != "blue" {
		t.Fatalf("tags: %v", res.Schema["tags"])
	}
	products, ok := res.Schema["products"].([]map[string]interface{})
	if !ok || len(products) != 2 {
		t.Fatalf("products: %v", res.Schema["products"])
	}
	if products[0]["name"] != "Widget" || products[0]["price"] != "9.99" {
		t.Fatalf("product 0: %v", products[0])
	}

	// Sanity: result.Schema is JSON-marshallable.
	if _, err := json.Marshal(res.Schema); err != nil {
		t.Fatalf("marshal: %v", err)
	}
}
