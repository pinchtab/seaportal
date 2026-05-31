package main_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary builds the seaportal CLI into a temp path and returns it.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "seaportal-test")

	// Locate the module root by walking up from the test file.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := wd
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		root = filepath.Dir(root)
	}

	cmd := exec.Command("go", "build", "-o", bin, "./cmd/seaportal")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, stderr.String())
	}
	return bin
}

const sampleHTML = `<!doctype html>
<html><head><title>Sample Page</title></head>
<body><article>
<h1>Welcome Heading</h1>
<p>This is a meaningful paragraph that should survive readability extraction without trouble. It has enough words to look like real content.</p>
<p>Another sentence to keep the article above the readability minimum threshold. Lorem ipsum dolor sit amet, consectetur adipiscing elit.</p>
</article></body></html>`

func TestCLI_StdinWithBaseURL(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "--base-url", "https://example.com", "--json", "-")
	cmd.Stdin = strings.NewReader(sampleHTML)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0, got %v; stderr=%s", err, stderr.String())
	}

	var res map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("json parse failed: %v\nstdout=%s", err, stdout.String())
	}
	if url, _ := res["url"].(string); url != "https://example.com" {
		t.Errorf("expected url=https://example.com, got %q", url)
	}
	if content, _ := res["content"].(string); !strings.Contains(strings.ToLower(content), "welcome heading") {
		t.Errorf("expected extracted content to contain heading; content=%q", content)
	}
}

func TestCLI_StdinMissingBaseURL(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "--json", "-")
	cmd.Stdin = strings.NewReader(sampleHTML)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit, got success; stdout=%s", stdout.String())
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("expected exit code 2, got %d", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), "base-url is required") {
		t.Errorf("expected stderr to mention 'base-url is required'; stderr=%s", stderr.String())
	}
}

func TestCLI_StdinEmpty(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "--base-url", "https://example.com", "--json", "-")
	cmd.Stdin = strings.NewReader("")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit, got success")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("expected exit code 2, got %d", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), "no HTML provided on stdin") {
		t.Errorf("expected stderr to mention empty stdin; stderr=%s", stderr.String())
	}
}

func TestCLI_StdinNoArgUsesStdin(t *testing.T) {
	// When no positional arg is provided at all, stdin mode should engage.
	bin := buildBinary(t)

	cmd := exec.Command(bin, "--base-url", "https://example.com", "--json")
	cmd.Stdin = strings.NewReader(sampleHTML)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0, got %v; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"url"`) {
		t.Errorf("expected JSON output with url; stdout=%s", stdout.String())
	}
}
