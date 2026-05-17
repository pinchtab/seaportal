package main_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestCLI_MCPSubcommand exec's the built binary with `mcp`, feeds a sequence
// of JSON-RPC requests on stdin (initialize → tools/list → tools/call
// parse_sitemap → EOF), and asserts the matching responses on stdout.
func TestCLI_MCPSubcommand(t *testing.T) {
	bin := buildBinary(t)

	// Synthetic sitemap server so parse_sitemap has something to fetch.
	mux := http.NewServeMux()
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/a</loc></url>
  <url><loc>https://example.com/b</loc></url>
</urlset>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cmd := exec.Command(bin, "mcp")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"parse_sitemap","arguments":{"url":%q}}}`, srv.URL+"/sitemap.xml"),
	}
	go func() {
		defer func() { _ = stdin.Close() }() // triggers clean EOF shutdown on server side
		for _, req := range requests {
			if _, err := io.WriteString(stdin, req+"\n"); err != nil {
				return
			}
		}
	}()

	// Read 3 line-delimited responses.
	reader := bufio.NewReaderSize(stdout, 1<<20)
	responses := make([]map[string]interface{}, 0, 3)
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 3; i++ {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				done <- fmt.Errorf("read response %d: %w", i, err)
				return
			}
			var r map[string]interface{}
			if err := json.Unmarshal(line, &r); err != nil {
				done <- fmt.Errorf("decode response %d: %w (%q)", i, err, string(line))
				return
			}
			responses = append(responses, r)
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			_ = cmd.Process.Kill()
			t.Fatalf("response loop: %v", err)
		}
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("timed out waiting for MCP responses")
	}

	// initialize
	initResult := responses[0]["result"].(map[string]interface{})
	if initResult["protocolVersion"] != "2024-11-05" {
		t.Errorf("initialize.protocolVersion = %v", initResult["protocolVersion"])
	}

	// tools/list — assert all 4 tool names present.
	listResult := responses[1]["result"].(map[string]interface{})
	rawTools, _ := listResult["tools"].([]interface{})
	gotNames := map[string]bool{}
	for _, raw := range rawTools {
		m := raw.(map[string]interface{})
		gotNames[m["name"].(string)] = true
	}
	for _, want := range []string{"fetch_url", "fetch_snapshot", "parse_sitemap", "parse_feed"} {
		if !gotNames[want] {
			t.Errorf("tools/list missing %q (got %v)", want, gotNames)
		}
	}

	// tools/call parse_sitemap
	callResult, ok := responses[2]["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("parse_sitemap response missing result: %+v", responses[2])
	}
	content := callResult["content"].([]interface{})
	if len(content) == 0 {
		t.Fatalf("parse_sitemap result has no content")
	}
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "https://example.com/a") || !strings.Contains(text, "https://example.com/b") {
		t.Errorf("parse_sitemap text missing expected URLs: %s", text)
	}

	// EOF (stdin already closed by writer goroutine) → server exits cleanly.
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	select {
	case err := <-waitErr:
		if err != nil {
			t.Errorf("seaportal mcp exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("seaportal mcp did not exit on EOF")
	}
}
