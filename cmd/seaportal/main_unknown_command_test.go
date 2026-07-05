package main_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestCLI_UnknownCommand(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "sitemp", "https://example.com")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit for bogus verb")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("want exit code 2, got %d", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), "unknown command: sitemp") {
		t.Errorf("stderr missing unknown-command message: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "seaportal help") {
		t.Errorf("stderr missing help pointer: %q", stderr.String())
	}
}

func TestCLI_RealURLStillExtracts(t *testing.T) {
	bin := buildBinary(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(sampleHTML))
	}))
	defer srv.Close()

	cmd := exec.Command(bin, "--allow-internal", "--json", srv.URL)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("real URL should not be treated as unknown command: %v\nstderr: %s", err, stderr.String())
	}
	if strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("real URL misclassified: %q", stderr.String())
	}
}
