package main_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestCLI_LocalFilePath_DoesNotPanic(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "./testdata/static/article-ldjson.html")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	combined := stdout.String() + stderr.String()
	if strings.Contains(combined, "runtime error: index out of range") {
		t.Fatalf("CLI panicked on local file path arg (regression):\nstdout=%s\nstderr=%s\nerr=%v",
			stdout.String(), stderr.String(), err)
	}
	if strings.Contains(combined, "panic:") {
		t.Fatalf("CLI panicked on local file path arg:\nstdout=%s\nstderr=%s\nerr=%v",
			stdout.String(), stderr.String(), err)
	}
}
