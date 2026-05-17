//go:build integration

// MCP protocol conformance test.
//
// Spawns `seaportal mcp` as a subprocess and drives JSON-RPC 2.0 over stdio
// with a hand-rolled mini-client. Locks in:
//
//   - initialize handshake shape (protocolVersion, serverInfo, capabilities)
//   - tools/list returns exactly 4 tools in registration order
//   - each tool's inputSchema is well-formed and has required:[url]
//   - no "optional" keyword leakage anywhere in tools/list
//   - JSON-RPC framing invariants (jsonrpc:"2.0", matching id, no
//     result+error mixing)
//
// Excluded from `./dev all` via the integration build tag.
// Run with: go test -tags=integration -run TestMCPConformance ./cmd/seaportal/
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// expectedToolOrder must match the RegisterTool call order in mcp.go.
var expectedToolOrder = []string{
	"fetch_url",
	"fetch_snapshot",
	"parse_sitemap",
	"parse_feed",
}

// ── mini JSON-RPC client ───────────────────────────────────────────────────

type rpcReq struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// mcpProc is a live subprocess + framed JSON-RPC pipes.
type mcpProc struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *bytes.Buffer
}

func (p *mcpProc) send(t *testing.T, id int, method string, params interface{}) {
	t.Helper()
	req := rpcReq{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	b = append(b, '\n')
	if _, err := p.stdin.Write(b); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

// recv reads one response line with a 5s timeout.
func (p *mcpProc) recv(t *testing.T) (rpcResp, []byte) {
	t.Helper()
	type result struct {
		line []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := p.stdout.ReadBytes('\n')
		ch <- result{line: line, err: err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("read response: %v (stderr=%q)", r.err, p.stderr.String())
		}
		var resp rpcResp
		if err := json.Unmarshal(r.line, &resp); err != nil {
			t.Fatalf("unmarshal response %q: %v", r.line, err)
		}
		return resp, r.line
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		t.Fatalf("timeout waiting for response (stderr=%q)", p.stderr.String())
		return rpcResp{}, nil
	}
}

// ── per-test subprocess plumbing ───────────────────────────────────────────

func startMCP(t *testing.T, binPath string) *mcpProc {
	t.Helper()
	cmd := exec.Command(binPath, "mcp")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	p := &mcpProc{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout), stderr: &stderr}
	t.Cleanup(func() {
		_ = stdin.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	})
	return p
}

// buildOnce compiles the seaportal binary into a tempdir and returns its path.
// Shared across all subtests via sync.Once.
var (
	buildOnceMu sync.Mutex
	builtPath   string
)

func ensureBinary(t *testing.T) string {
	t.Helper()
	buildOnceMu.Lock()
	defer buildOnceMu.Unlock()
	if builtPath != "" {
		return builtPath
	}
	tmpDir, err := os.MkdirTemp("", "seaportal-conformance-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	binName := "seaportal"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	p := filepath.Join(tmpDir, binName)
	buildCmd := exec.Command("go", "build", "-o", p, "./cmd/seaportal/")
	buildCmd.Dir = repoRoot(t)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	builtPath = p
	return p
}

// ── tests ──────────────────────────────────────────────────────────────────

func TestMCPConformance(t *testing.T) {
	binPath := ensureBinary(t)

	t.Run("Initialize_HandshakeSucceeds", func(t *testing.T) {
		p := startMCP(t, binPath)
		p.send(t, 1, "initialize", map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"clientInfo":      map[string]interface{}{"name": "conformance", "version": "0"},
		})
		resp, raw := p.recv(t)
		assertFraming(t, resp, raw, 1)

		var result struct {
			ProtocolVersion string                 `json:"protocolVersion"`
			Capabilities    map[string]interface{} `json:"capabilities"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("unmarshal initialize result: %v", err)
		}
		if result.ProtocolVersion == "" {
			t.Errorf("missing protocolVersion")
		}
		if result.ServerInfo.Name == "" {
			t.Errorf("missing serverInfo.name")
		}
		if result.ServerInfo.Version == "" {
			t.Errorf("missing serverInfo.version")
		}
		if result.Capabilities == nil {
			t.Errorf("missing capabilities")
		}
	})

	t.Run("ToolsList_ReturnsExpectedFour", func(t *testing.T) {
		p := startMCP(t, binPath)
		p.send(t, 1, "initialize", map[string]interface{}{"protocolVersion": "2024-11-05"})
		_, _ = p.recv(t)

		p.send(t, 2, "tools/list", nil)
		resp, raw := p.recv(t)
		assertFraming(t, resp, raw, 2)

		// Log the observed payload on first run for lock-in visibility.
		t.Logf("tools/list payload: %s", string(raw))

		var result struct {
			Tools []struct {
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				InputSchema map[string]interface{} `json:"inputSchema"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("unmarshal tools/list: %v", err)
		}
		if len(result.Tools) != len(expectedToolOrder) {
			t.Fatalf("got %d tools, want %d", len(result.Tools), len(expectedToolOrder))
		}
		for i, want := range expectedToolOrder {
			if result.Tools[i].Name != want {
				t.Errorf("tool[%d] = %q, want %q", i, result.Tools[i].Name, want)
			}
			if result.Tools[i].Description == "" {
				t.Errorf("tool %q missing description", want)
			}
		}
	})

	t.Run("ToolsList_InputSchemasWellFormed", func(t *testing.T) {
		p := startMCP(t, binPath)
		p.send(t, 1, "initialize", map[string]interface{}{"protocolVersion": "2024-11-05"})
		_, _ = p.recv(t)

		p.send(t, 2, "tools/list", nil)
		resp, raw := p.recv(t)
		assertFraming(t, resp, raw, 2)

		// No "optional" keyword leakage anywhere in tools/list.
		// (Standard JSON-Schema-ish MCP uses required-array, not "optional".)
		if bytes.Contains(bytes.ToLower(raw), []byte(`"optional"`)) {
			t.Errorf(`"optional" keyword leaked into tools/list output: %s`, raw)
		}

		var result struct {
			Tools []struct {
				Name        string                 `json:"name"`
				InputSchema map[string]interface{} `json:"inputSchema"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("unmarshal tools/list: %v", err)
		}
		for _, tl := range result.Tools {
			s := tl.InputSchema
			if s == nil {
				t.Errorf("tool %q: missing inputSchema", tl.Name)
				continue
			}
			if typ, _ := s["type"].(string); typ != "object" {
				t.Errorf("tool %q: inputSchema.type = %v, want \"object\"", tl.Name, s["type"])
			}
			props, ok := s["properties"].(map[string]interface{})
			if !ok || len(props) == 0 {
				t.Errorf("tool %q: inputSchema.properties missing or empty", tl.Name)
			}
			// Locked-in observation (2026-05-17): every tool currently sets
			// required:["url"]. If this ever changes intentionally, update
			// here and document the rationale.
			req, ok := s["required"].([]interface{})
			if !ok {
				t.Errorf("tool %q: inputSchema.required missing or wrong type", tl.Name)
				continue
			}
			if len(req) != 1 || req[0] != "url" {
				t.Errorf("tool %q: required = %v, want [url]", tl.Name, req)
			}
			if _, hasURL := props["url"]; !hasURL {
				t.Errorf("tool %q: properties missing \"url\"", tl.Name)
			}
		}
	})

	t.Run("JSONRPCFraming_NoMixedResultError", func(t *testing.T) {
		p := startMCP(t, binPath)

		// initialize → success path
		p.send(t, 1, "initialize", map[string]interface{}{"protocolVersion": "2024-11-05"})
		respOK, rawOK := p.recv(t)
		assertFraming(t, respOK, rawOK, 1)
		if respOK.Result == nil {
			t.Errorf("expected non-nil result on initialize")
		}
		if respOK.Error != nil {
			t.Errorf("unexpected error on initialize: %+v", respOK.Error)
		}

		// unknown method → error path; must still be valid JSON-RPC, id matched,
		// and must NOT include a result alongside the error.
		p.send(t, 42, "nonexistent/method", nil)
		respErr, rawErr := p.recv(t)
		assertFraming(t, respErr, rawErr, 42)
		if respErr.Error == nil {
			t.Errorf("expected error for unknown method, got: %s", rawErr)
		}
		// Verify no `"result":` key sits next to `"error":` in the raw line.
		// (json.RawMessage would be empty if absent, but be paranoid about
		// raw key presence — the spec forbids both at once.)
		if respErr.Error != nil && hasResultKey(rawErr) {
			t.Errorf("response contains both result and error: %s", rawErr)
		}
	})
}

// assertFraming verifies JSON-RPC 2.0 envelope invariants: jsonrpc == "2.0",
// id matches the request id, and result/error are not both set.
func assertFraming(t *testing.T, r rpcResp, raw []byte, wantID int) {
	t.Helper()
	if r.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want \"2.0\" (raw=%s)", r.JSONRPC, raw)
	}
	var gotID int
	if err := json.Unmarshal(r.ID, &gotID); err != nil {
		t.Errorf("id not an int: %s (raw=%s)", r.ID, raw)
	} else if gotID != wantID {
		t.Errorf("id = %d, want %d", gotID, wantID)
	}
	if r.Result != nil && r.Error != nil {
		t.Errorf("response has both result and error: %s", raw)
	}
}

// hasResultKey returns true iff the raw JSON object contains a top-level
// "result" key. Lightweight scan — sufficient for one-line responses since
// the server never emits nested "result" strings in error payloads.
func hasResultKey(raw []byte) bool {
	// Tolerate either ordering. Quick string-level check is fine for tests.
	s := string(raw)
	return strings.Contains(s, `"result":`) && !strings.Contains(s, `"result":null`)
}

// repoRoot is defined in mcp_coldstart_test.go (same package, same build tag).
