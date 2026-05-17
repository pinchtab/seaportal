// Package mcp implements a minimal Model Context Protocol (MCP) server over
// JSON-RPC 2.0 line-delimited stdio. It is intentionally tiny: just enough of
// the protocol to satisfy "initialize", "tools/list" and "tools/call" so that
// editors (Claude Code / Cursor / VS Code) can drive seaportal's library
// functions as tools.
//
// One JSON-RPC request per stdin line; one JSON-RPC response per stdout line.
// Notifications (no id) produce no response. Requests are handled serially
// (V1 — no concurrency).
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// ProtocolVersion is the MCP spec revision this server advertises.
const ProtocolVersion = "2024-11-05"

// Standard JSON-RPC 2.0 error codes.
const (
	codeParseError     = -32700
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// ToolHandler is invoked when a client calls a registered tool. The returned
// string is wrapped in `{content: [{type:"text", text: ...}]}` for the client.
type ToolHandler func(args map[string]interface{}) (string, error)

type tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	handler     ToolHandler
}

// Server is a minimal MCP server. Construct with NewServer, register tools,
// then call ServeStdio.
type Server struct {
	mu      sync.Mutex
	tools   map[string]tool
	ordered []string // preserves registration order for tools/list
	name    string
	version string
}

// NewServer returns an empty MCP server with default identity.
func NewServer() *Server {
	return &Server{
		tools:   map[string]tool{},
		name:    "seaportal",
		version: "dev",
	}
}

// SetIdentity overrides the serverInfo reported during initialize.
func (s *Server) SetIdentity(name, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name != "" {
		s.name = name
	}
	if version != "" {
		s.version = version
	}
}

// RegisterTool adds a tool to the registry. Re-registering the same name
// overwrites the previous entry.
func (s *Server) RegisterTool(name, description string, inputSchema map[string]interface{}, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tools[name]; !exists {
		s.ordered = append(s.ordered, name)
	}
	s.tools[name] = tool{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
		handler:     handler,
	}
}

// ServeStdio runs the server over the process's stdin/stdout.
func (s *Server) ServeStdio(ctx context.Context) error {
	return s.serve(ctx, os.Stdin, os.Stdout)
}

// ── JSON-RPC envelopes ─────────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

var nullID = json.RawMessage("null")

// serve runs the read/respond loop. Exposed (lowercase) for tests via pipes.
func (s *Server) serve(_ context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	// 16 MB max line — extracted Markdown / sitemap JSON can be large.
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	enc := json.NewEncoder(out)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		resp := s.handleRequest(line)
		if resp == nil {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func errorResponse(id json.RawMessage, code int, msg string) *rpcResponse {
	if len(id) == 0 {
		id = nullID
	}
	return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

func okResponse(id json.RawMessage, result interface{}) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

// isNotification reports whether the raw id field marks the request as a
// JSON-RPC notification (absent id). JSON `null` counts as a regular request
// per the spec, though responses to it are still discouraged.
func isNotification(id json.RawMessage) bool {
	return len(id) == 0
}

// handleRequest parses one line and returns the response envelope (or nil for
// notifications). It never panics: handler panics are recovered.
func (s *Server) handleRequest(line []byte) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return errorResponse(nullID, codeParseError, "parse error: "+err.Error())
	}

	switch req.Method {
	case "initialize":
		if isNotification(req.ID) {
			return nil
		}
		return okResponse(req.ID, map[string]interface{}{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    s.name,
				"version": s.version,
			},
		})

	case "notifications/initialized", "initialized":
		// MCP clients typically send this notification after initialize.
		return nil

	case "tools/list":
		if isNotification(req.ID) {
			return nil
		}
		s.mu.Lock()
		list := make([]map[string]interface{}, 0, len(s.ordered))
		for _, name := range s.ordered {
			t := s.tools[name]
			list = append(list, map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.InputSchema,
			})
		}
		s.mu.Unlock()
		return okResponse(req.ID, map[string]interface{}{"tools": list})

	case "tools/call":
		if isNotification(req.ID) {
			return nil
		}
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return errorResponse(req.ID, codeInvalidParams, "invalid params: "+err.Error())
			}
		}
		if params.Name == "" {
			return errorResponse(req.ID, codeInvalidParams, "missing tool name")
		}
		s.mu.Lock()
		t, ok := s.tools[params.Name]
		s.mu.Unlock()
		if !ok {
			return errorResponse(req.ID, codeInvalidParams, "tool not found: "+params.Name)
		}
		if params.Arguments == nil {
			params.Arguments = map[string]interface{}{}
		}
		text, panicked, err := s.callHandler(t.handler, params.Arguments)
		if panicked {
			return errorResponse(req.ID, codeInternalError, err.Error())
		}
		if err != nil {
			// Return a tool-level error as a successful result with isError=true,
			// per the MCP spec. JSON-RPC error reserved for protocol failures.
			return okResponse(req.ID, map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": err.Error()},
				},
				"isError": true,
			})
		}
		return okResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": text},
			},
		})

	default:
		if isNotification(req.ID) {
			return nil
		}
		return errorResponse(req.ID, codeMethodNotFound, "method not found: "+req.Method)
	}
}

// callHandler invokes a tool handler, converting panics into a flagged error
// so the server stays alive and the caller can map them to JSON-RPC -32603.
func (s *Server) callHandler(h ToolHandler, args map[string]interface{}) (out string, panicked bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()
	out, err = h(args)
	return out, false, err
}
