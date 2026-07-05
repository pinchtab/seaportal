package main_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestCLI_VersionVerb(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("seaportal version: %v\nstderr: %s", err, stderr.String())
	}

	out := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(out, "seaportal ") || len(out) <= len("seaportal ") {
		t.Errorf("want %q output like 'seaportal <version>', got %q", "version", out)
	}

	flagCmd := exec.Command(bin, "--version")
	var flagOut bytes.Buffer
	flagCmd.Stdout = &flagOut
	if err := flagCmd.Run(); err != nil {
		t.Fatalf("seaportal --version: %v", err)
	}
	if got := strings.TrimSpace(flagOut.String()); got != out {
		t.Errorf("verb and flag disagree: version=%q --version=%q", out, got)
	}
}

func TestCLI_HelpListsVersion(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "help")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("seaportal help: %v", err)
	}
	usage := stdout.String() + stderr.String()
	if !strings.Contains(usage, "seaportal version") {
		t.Errorf("help output does not list the version command:\n%s", usage)
	}
}
