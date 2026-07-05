package main_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestCLI_HelpVerbToStdout(t *testing.T) {
	bin := buildBinary(t)

	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		cmd := exec.Command(bin, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("seaportal %v should exit 0: %v\nstderr: %s", args, err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Usage:") {
			t.Errorf("seaportal %v: usage should be on stdout, got stdout=%q stderr=%q",
				args, stdout.String(), stderr.String())
		}
		if strings.Contains(stderr.String(), "Usage:") {
			t.Errorf("seaportal %v: usage leaked to stderr: %q", args, stderr.String())
		}
	}
}

func TestCLI_ErrorUsageToStderr(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "sitemap")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("seaportal sitemap with no URL should exit non-zero")
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("want exit code 2, got %v", err)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("error usage should be on stderr, got stderr=%q", stderr.String())
	}
	if strings.Contains(stdout.String(), "Usage:") {
		t.Errorf("error usage leaked to stdout: %q", stdout.String())
	}
}
