package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/pinchtab/seaportal/internal/mcp"
)

// runToolRequests registers the tool surface on a fresh mcp.Server and drives
// it over ServeStdio with os.Stdin/os.Stdout temporarily swapped for pipes —
// the only wire into the server from outside package mcp. Returns one parsed
// response per stdout line. Not parallel-safe (mutates process globals).
func runToolRequests(t *testing.T, requests ...string) []map[string]interface{} {
	t.Helper()
	srv := mcp.NewServer()
	Register(srv)

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = inR, outW

	done := make(chan error, 1)
	go func() { done <- srv.ServeStdio(context.Background()) }()

	for _, req := range requests {
		if _, err := inW.WriteString(req + "\n"); err != nil {
			t.Fatalf("write request: %v", err)
		}
	}
	_ = inW.Close()
	if err := <-done; err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	os.Stdin, os.Stdout = oldIn, oldOut
	_ = outW.Close()

	raw, err := io.ReadAll(outR)
	if err != nil {
		t.Fatalf("read responses: %v", err)
	}
	_ = outR.Close()
	_ = inR.Close()

	var responses []map[string]interface{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	for dec.More() {
		var r map[string]interface{}
		if err := dec.Decode(&r); err != nil {
			t.Fatalf("decode response: %v (raw=%q)", err, raw)
		}
		responses = append(responses, r)
	}
	return responses
}

// expectedTools is the wire contract: five tools, registration order.
var expectedTools = []struct {
	name       string
	schemaKeys []string
}{
	{"fetch_url", []string{"url", "dedupe", "fast", "with_links", "with_images", "with_tables", "with_comments", "max_tokens"}},
	{"fetch_snapshot", []string{"url", "filter", "max_tokens", "allow_internal"}},
	{"parse_sitemap", []string{"url", "max_depth", "max_urls", "allow_internal"}},
	{"parse_feed", []string{"url", "max_items", "allow_internal"}},
	{"scrape_site", []string{"base_url", "max_pages", "max_per_pattern", "full", "include_patterns", "exclude_patterns", "sample_strategy", "with_performance", "respect_robots", "timeout_seconds", "user_agent", "allow_internal"}},
}

func TestRegister_FiveToolsInOrderWithSchemas(t *testing.T) {
	resp := runToolRequests(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}
	result, ok := resp[0]["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing result: %+v", resp[0])
	}
	list, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatalf("missing tools list: %+v", result)
	}
	if len(list) != len(expectedTools) {
		t.Fatalf("tool count = %d, want %d", len(list), len(expectedTools))
	}

	for i, want := range expectedTools {
		tool, ok := list[i].(map[string]interface{})
		if !ok {
			t.Fatalf("tools[%d] not an object: %#v", i, list[i])
		}
		if got := tool["name"]; got != want.name {
			t.Errorf("tools[%d].name = %v, want %s (registration order is wire contract)", i, got, want.name)
		}
		if desc, _ := tool["description"].(string); desc == "" {
			t.Errorf("%s: empty description", want.name)
		}

		schema, ok := tool["inputSchema"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s: missing inputSchema", want.name)
		}
		if typ, _ := schema["type"].(string); typ != "object" {
			t.Errorf("%s: schema type = %v, want object", want.name, schema["type"])
		}
		props, ok := schema["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s: missing properties", want.name)
		}
		if len(props) != len(want.schemaKeys) {
			t.Errorf("%s: %d properties, want %d (%v)", want.name, len(props), len(want.schemaKeys), props)
		}
		for _, key := range want.schemaKeys {
			if _, ok := props[key]; !ok {
				t.Errorf("%s: schema missing property %q", want.name, key)
			}
		}

		required, ok := schema["required"].([]interface{})
		if !ok || len(required) != 1 {
			t.Fatalf("%s: required = %#v, want exactly one entry", want.name, schema["required"])
		}
		wantRequired := "url"
		if want.name == "scrape_site" {
			wantRequired = "base_url"
		}
		if required[0] != wantRequired {
			t.Errorf("%s: required[0] = %v, want %s", want.name, required[0], wantRequired)
		}
	}
}

// callToolError extracts the isError text content from a tools/call response.
func callToolError(t *testing.T, resp map[string]interface{}) string {
	t.Helper()
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing result: %+v", resp)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Fatalf("expected isError=true, got %+v", result)
	}
	content, ok := result["content"].([]interface{})
	if !ok || len(content) != 1 {
		t.Fatalf("expected one content block: %+v", result)
	}
	block, _ := content[0].(map[string]interface{})
	text, _ := block["text"].(string)
	return text
}

func TestToolCall_MissingRequiredArgIsToolError(t *testing.T) {
	resp := runToolRequests(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fetch_url","arguments":{}}}`)
	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}
	if got := callToolError(t, resp[0]); got != "missing required argument: url" {
		t.Errorf("error text = %q", got)
	}
}

func TestToolCall_ScrapeSiteRejectsUnknownStrategy(t *testing.T) {
	resp := runToolRequests(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"scrape_site","arguments":{"base_url":"https://example.com","sample_strategy":"bogus"}}}`)
	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}
	got := callToolError(t, resp[0])
	want := `unknown sample_strategy "bogus" (want balanced|random|priority)`
	if got != want {
		t.Errorf("error text = %q, want %q", got, want)
	}
}
