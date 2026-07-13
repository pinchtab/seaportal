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

const ProtocolVersion = "2024-11-05"

const (
	codeParseError     = -32700
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

type ToolHandler func(ctx context.Context, args map[string]interface{}) (string, error)

type tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	handler     ToolHandler
}

type Server struct {
	mu      sync.Mutex
	tools   map[string]tool
	ordered []string
	name    string
	version string
}

func NewServer() *Server {
	return &Server{
		tools:   map[string]tool{},
		name:    "seaportal",
		version: "dev",
	}
}

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

func (s *Server) ServeStdio(ctx context.Context) error {
	return s.serve(ctx, os.Stdin, os.Stdout)
}

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

func (s *Server) serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	enc := json.NewEncoder(out)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		resp := s.handleRequest(ctx, line)
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

func isNotification(id json.RawMessage) bool {
	return len(id) == 0
}

func (s *Server) handleRequest(ctx context.Context, line []byte) *rpcResponse {
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
		text, panicked, err := s.callHandler(ctx, t.handler, params.Arguments)
		if panicked {
			return errorResponse(req.ID, codeInternalError, err.Error())
		}
		if err != nil {
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

func (s *Server) callHandler(ctx context.Context, h ToolHandler, args map[string]interface{}) (out string, panicked bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()
	out, err = h(ctx, args)
	return out, false, err
}
