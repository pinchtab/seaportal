package engine

import "testing"

func TestLoadSchema_Example(t *testing.T) {
	schema, err := LoadSchema("../../testdata/schema/example.yaml")
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	for _, want := range []string{"title", "author", "publish_date"} {
		if _, ok := schema.Fields[want]; !ok {
			t.Errorf("schema missing %q field; got %+v", want, schema)
		}
	}
	if schema.Fields["publish_date"].Attr != "datetime" {
		t.Errorf("publish_date.Attr = %q, want %q", schema.Fields["publish_date"].Attr, "datetime")
	}
}
