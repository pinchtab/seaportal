package main_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// regression: cli-file-path-panic
//
// `seaportal ./some/file.html` used to panic with
// `index out of range [1] with length 1` because cmd/seaportal/main.go
// constructed the renders/ output slug via `strings.Split(arg, "//")[1]`
// — file paths have no `//` so the slice was 1-element and `[1]` panicked.
// The guard now falls back to a synthetic `local` slug.
//
// The test only asserts NO PANIC (exit code != 2 from runtime panic).
// End-to-end file:// fetching is intentionally not supported; the engine
// will surface a normal fetch error, which is fine — that's a separate
// feature task.
func TestCLI_LocalFilePath_DoesNotPanic(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "./testdata/static/article-ldjson.html")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	// We don't care about exit code — a fetch error is expected since the
	// engine doesn't support file:// URIs. We only assert no Go runtime
	// panic, which would produce a "runtime error:" line on stderr.
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
