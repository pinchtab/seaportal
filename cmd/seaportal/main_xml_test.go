package main_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestCLI_XMLOutput(t *testing.T) {
	bin := buildBinary(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(sampleHTML))
	}))
	defer srv.Close()

	cmd := exec.Command(bin, "--xml", srv.URL)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("seaportal --xml: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "<?xml") {
		t.Errorf("output should start with <?xml, got: %.80q", out)
	}
	if !strings.Contains(out, `xmlns="http://www.tei-c.org/ns/1.0"`) {
		t.Errorf("output missing TEI namespace: %s", out)
	}
	if !strings.Contains(out, "<teiHeader>") {
		t.Errorf("output missing <teiHeader>: %s", out)
	}
}

func TestCLI_XMLAndJSONErrors(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "--xml", "--json", "https://example.com")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit when --xml and --json are combined")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("want exit code 2, got %d", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Errorf("stderr should mention 'mutually exclusive', got: %s", stderr.String())
	}
}
