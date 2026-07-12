package main_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"
)

// TestCLI_MCPScrapeSite exec's `seaportal mcp` and drives a JSON-RPC session
// (initialize → tools/list → tools/call scrape_site) against a local HTTP
// fixture, asserting scrape_site is advertised and returns a ScrapeResult.
func TestCLI_MCPScrapeSite(t *testing.T) {
	bin := buildBinary(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			http.NotFound(w, r)
			return
		}
		body := `<html><head><title>` + r.URL.Path + `</title></head><body><h1>` + r.URL.Path + `</h1>`
		if r.URL.Path == "/" {
			body += `<a href="/about">About</a><a href="/blog/1">Post</a>`
		}
		body += `<p>Body content for extraction.</p></body></html>`
		_, _ = w.Write([]byte(body))
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
	defer func() { _ = cmd.Process.Kill() }()

	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"scrape_site","arguments":{"base_url":%q,"max_pages":3,"allow_internal":true}}}`, srv.URL),
	}
	go func() {
		defer func() { _ = stdin.Close() }()
		for _, req := range requests {
			if _, err := io.WriteString(stdin, req+"\n"); err != nil {
				return
			}
		}
	}()

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
			t.Fatalf("response loop: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("timed out waiting for MCP responses")
	}

	// tools/list advertises scrape_site with a schema.
	listResult := responses[1]["result"].(map[string]interface{})
	rawTools, _ := listResult["tools"].([]interface{})
	var scrapeTool map[string]interface{}
	for _, raw := range rawTools {
		m := raw.(map[string]interface{})
		if m["name"] == "scrape_site" {
			scrapeTool = m
		}
	}
	if scrapeTool == nil {
		t.Fatal("tools/list did not advertise scrape_site")
	}
	if schema, _ := scrapeTool["inputSchema"].(map[string]interface{}); schema == nil || schema["properties"] == nil {
		t.Errorf("scrape_site missing input schema: %+v", scrapeTool)
	}

	// tools/call scrape_site returns a ScrapeResult for the fixture.
	callResult, ok := responses[2]["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("scrape_site response missing result: %+v", responses[2])
	}
	content := callResult["content"].([]interface{})
	if len(content) == 0 {
		t.Fatal("scrape_site result has no content")
	}
	text := content[0].(map[string]interface{})["text"].(string)

	var res struct {
		Site struct {
			BaseURL string `json:"baseURL"`
		} `json:"site"`
		Pages []map[string]any `json:"pages"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		t.Fatalf("scrape_site content is not ScrapeResult JSON: %v\n%s", err, text)
	}
	if res.Site.BaseURL != srv.URL {
		t.Errorf("site.baseURL = %s, want %s", res.Site.BaseURL, srv.URL)
	}
	if len(res.Pages) == 0 {
		t.Error("scrape_site returned no pages")
	}
}
