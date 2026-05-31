package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// runRequests pipes the given JSON-RPC request lines through a fresh server
// and returns parsed response objects (one per non-empty stdout line).
func runRequests(t *testing.T, s *Server, requests ...string) []map[string]interface{} {
	t.Helper()
	in := strings.NewReader(strings.Join(requests, "\n") + "\n")
	var out bytes.Buffer
	if err := s.serve(context.Background(), in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var responses []map[string]interface{}
	for _, line := range bytes.Split(bytes.TrimRight(out.Bytes(), "\n"), []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r map[string]interface{}
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatalf("decode response %q: %v", string(line), err)
		}
		responses = append(responses, r)
	}
	return responses
}

func TestMCPServer_Initialize(t *testing.T) {
	s := NewServer()
	resp := runRequests(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}
	result, ok := resp[0]["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing result: %+v", resp[0])
	}
	if v, _ := result["protocolVersion"].(string); v != ProtocolVersion {
		t.Errorf("protocolVersion = %q, want %q", v, ProtocolVersion)
	}
	caps, ok := result["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing capabilities")
	}
	if _, ok := caps["tools"]; !ok {
		t.Errorf("expected tools capability advertised")
	}
	info, _ := result["serverInfo"].(map[string]interface{})
	if info["name"] != "seaportal" {
		t.Errorf("serverInfo.name = %v, want seaportal", info["name"])
	}
}

func TestMCPServer_ListTools(t *testing.T) {
	s := NewServer()
	schema := map[string]interface{}{"type": "object"}
	s.RegisterTool("alpha", "alpha tool", schema, func(map[string]interface{}) (string, error) { return "a", nil })
	s.RegisterTool("beta", "beta tool", schema, func(map[string]interface{}) (string, error) { return "b", nil })

	resp := runRequests(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}
	result := resp[0]["result"].(map[string]interface{})
	tools, _ := result["tools"].([]interface{})
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, raw := range tools {
		m := raw.(map[string]interface{})
		names[m["name"].(string)] = true
		if _, ok := m["inputSchema"]; !ok {
			t.Errorf("tool %v missing inputSchema", m["name"])
		}
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("missing expected tool names: %v", names)
	}
}

func TestMCPServer_CallTool_Echo(t *testing.T) {
	s := NewServer()
	s.RegisterTool("echo", "echoes args back as JSON", map[string]interface{}{"type": "object"},
		func(args map[string]interface{}) (string, error) {
			b, _ := json.Marshal(args)
			return string(b), nil
		},
	)

	resp := runRequests(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"hello":"world"}}}`)
	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}
	result := resp[0]["result"].(map[string]interface{})
	content := result["content"].([]interface{})
	if len(content) == 0 {
		t.Fatalf("empty content")
	}
	first := content[0].(map[string]interface{})
	if first["type"] != "text" {
		t.Errorf("content[0].type = %v, want text", first["type"])
	}
	text := first["text"].(string)
	if !strings.Contains(text, `"hello":"world"`) {
		t.Errorf("echoed text missing args: %q", text)
	}
}

func TestMCPServer_UnknownTool(t *testing.T) {
	s := NewServer()
	resp := runRequests(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nope"}}`)
	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}
	errObj, ok := resp[0]["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error, got %+v", resp[0])
	}
	if code, _ := errObj["code"].(float64); int(code) != codeInvalidParams {
		t.Errorf("code = %v, want %d", code, codeInvalidParams)
	}
}

func TestMCPServer_UnknownMethod(t *testing.T) {
	s := NewServer()
	resp := runRequests(t, s, `{"jsonrpc":"2.0","id":5,"method":"does/not/exist"}`)
	if len(resp) != 1 {
		t.Fatalf("expected 1 response")
	}
	errObj := resp[0]["error"].(map[string]interface{})
	if code, _ := errObj["code"].(float64); int(code) != codeMethodNotFound {
		t.Errorf("code = %v, want %d", code, codeMethodNotFound)
	}
}

func TestMCPServer_MalformedJSON(t *testing.T) {
	s := NewServer()
	resp := runRequests(t, s, `{not json`)
	if len(resp) != 1 {
		t.Fatalf("expected 1 response")
	}
	errObj := resp[0]["error"].(map[string]interface{})
	if code, _ := errObj["code"].(float64); int(code) != codeParseError {
		t.Errorf("code = %v, want %d", code, codeParseError)
	}
}

func TestMCPServer_StdinEOF(t *testing.T) {
	s := NewServer()
	var out bytes.Buffer
	if err := s.serve(context.Background(), strings.NewReader(""), &out); err != nil {
		t.Fatalf("serve on empty input returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected no output, got %q", out.String())
	}
}

func TestMCPServer_HandlerPanic(t *testing.T) {
	s := NewServer()
	s.RegisterTool("boom", "panics", map[string]interface{}{"type": "object"},
		func(map[string]interface{}) (string, error) { panic("kaboom") },
	)
	s.RegisterTool("ok", "fine", map[string]interface{}{"type": "object"},
		func(map[string]interface{}) (string, error) { return "still alive", nil },
	)

	resp := runRequests(t, s,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"boom","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"ok","arguments":{}}}`,
	)
	if len(resp) != 2 {
		t.Fatalf("expected 2 responses (server should survive panic), got %d", len(resp))
	}
	errObj, ok := resp[0]["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("first response should be error, got %+v", resp[0])
	}
	if code, _ := errObj["code"].(float64); int(code) != codeInternalError {
		t.Errorf("panic code = %v, want %d", code, codeInternalError)
	}
	result := resp[1]["result"].(map[string]interface{})
	if result == nil {
		t.Fatalf("second response missing result — server may have died")
	}
}

func TestMCPServer_Notification(t *testing.T) {
	s := NewServer()
	s.RegisterTool("echo", "", map[string]interface{}{"type": "object"},
		func(args map[string]interface{}) (string, error) { return "x", nil },
	)
	// No id field => notification. No response expected.
	resp := runRequests(t, s, `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"echo","arguments":{}}}`)
	if len(resp) != 0 {
		t.Errorf("expected no response to notification, got %d", len(resp))
	}
}
