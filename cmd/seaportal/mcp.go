package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/pinchtab/seaportal/internal/mcp"
	"github.com/pinchtab/seaportal/internal/mcp/tools"
)

func runMCP(ctx context.Context, _ []string) {
	srv := mcp.NewServer()
	srv.SetIdentity("seaportal", version)
	tools.Register(srv)

	if err := srv.ServeStdio(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "mcp server error:", err)
		os.Exit(1)
	}
}
