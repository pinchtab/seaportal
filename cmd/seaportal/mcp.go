package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/pinchtab/seaportal/internal/mcp"
	"github.com/pinchtab/seaportal/internal/mcp/tools"
)

// runMCP starts seaportal as an MCP (Model Context Protocol) server over
// JSON-RPC 2.0 line-delimited stdio. The tool surface (definitions, schemas,
// arg parsing, guardrails) lives in internal/mcp/tools; this shim only wires
// identity and transport. No flags are accepted: configuration flows through
// MCP tool arguments.
func runMCP(_ []string) {
	srv := mcp.NewServer()
	srv.SetIdentity("seaportal", version)
	tools.Register(srv)

	if err := srv.ServeStdio(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "mcp server error:", err)
		os.Exit(1)
	}
}
