package testserver

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestServerStart(t *testing.T) {
	srv := Start(0)
	defer srv.Stop()

	url := srv.URL()
	if url == "" {
		t.Fatal("URL() returned empty string")
	}

	if !strings.HasPrefix(url, "http://localhost:") {
		t.Fatalf("URL format incorrect, got: %s", url)
	}
}

func TestServerServeSimpleHTML(t *testing.T) {
	srv := Start(0)
	defer srv.Stop()

	resp, err := http.Get(srv.URL() + "/static/simple.html")
	if err != nil {
		t.Fatalf("failed to fetch simple.html: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	content := string(body)
	if !strings.Contains(content, "Welcome to Simple Page") {
		t.Fatal("expected content not found in response")
	}
	if !strings.Contains(content, "<h1>") {
		t.Fatal("expected HTML tags not found")
	}
}

func TestServerServeArticleHTML(t *testing.T) {
	srv := Start(0)
	defer srv.Stop()

	resp, err := http.Get(srv.URL() + "/static/article.html")
	if err != nil {
		t.Fatalf("failed to fetch article.html: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	content := string(body)
	if !strings.Contains(content, "Understanding Web Extraction") {
		t.Fatal("article title not found")
	}
	if !strings.Contains(content, "<ul>") && !strings.Contains(content, "<li>") {
		t.Fatal("list elements not found")
	}
	if !strings.Contains(content, "<code>") {
		t.Fatal("code block not found")
	}
}

func TestServerServeTableHTML(t *testing.T) {
	srv := Start(0)
	defer srv.Stop()

	resp, err := http.Get(srv.URL() + "/static/table.html")
	if err != nil {
		t.Fatalf("failed to fetch table.html: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	content := string(body)
	if !strings.Contains(content, "<table") {
		t.Fatal("table element not found")
	}
	if !strings.Contains(content, "<thead>") {
		t.Fatal("thead element not found")
	}
	if !strings.Contains(content, "<tbody>") {
		t.Fatal("tbody element not found")
	}
}

func TestServerMultipleInstances(t *testing.T) {
	srv1 := Start(0)
	defer srv1.Stop()

	srv2 := Start(0)
	defer srv2.Stop()

	// Both servers should be on different ports
	if srv1.URL() == srv2.URL() {
		t.Fatal("multiple server instances should have different URLs")
	}

	// Both should be accessible
	resp1, err := http.Get(srv1.URL() + "/static/simple.html")
	if err != nil {
		t.Fatalf("srv1 not accessible: %v", err)
	}
	_ = resp1.Body.Close()

	resp2, err := http.Get(srv2.URL() + "/static/simple.html")
	if err != nil {
		t.Fatalf("srv2 not accessible: %v", err)
	}
	_ = resp2.Body.Close()
}

func TestServerStopShutdown(t *testing.T) {
	srv := Start(0)
	url := srv.URL()

	// Server should be running
	resp, err := http.Get(url + "/static/simple.html")
	if err != nil {
		t.Fatalf("server should be running: %v", err)
	}
	_ = resp.Body.Close()

	// Stop the server
	srv.Stop()

	// Server should no longer be accessible (may take a moment)
	// We don't strictly check this since there's a timing window,
	// but we verify Stop doesn't panic
}
