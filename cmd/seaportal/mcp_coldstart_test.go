//go:build integration

// MCP cold-start performance gate.
//
// Builds the seaportal binary once into a tempdir, then spawns
// `seaportal mcp` three times. For each run, measures wall-clock from
// cmd.Start() to the first byte of the `initialize` JSON-RPC response on
// stdout. Asserts that the median of the three samples is at or under the
// locked-in threshold.
//
// Run with: go test -tags=integration -run TestMCP_ColdStart ./cmd/seaportal/
//
// Excluded from the default ./dev all run via the `integration` build tag.
package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"
)

// coldStartBudget is the maximum allowed median cold-start time for
// `seaportal mcp` (process spawn → first byte of initialize response on
// stdout). Backlog target: 350 ms. Do NOT silently widen — if this fails,
// investigate the regression rather than relax the gate.
const coldStartBudget = 350 * time.Millisecond

const initializeRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"coldstart-test","version":"0"}}}` + "\n"

func TestMCP_ColdStartUnder350ms(t *testing.T) {
	// 1) Build the binary once into a tempdir.
	tmpDir, err := os.MkdirTemp("", "seaportal-coldstart-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	binName := "seaportal"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tmpDir, binName)

	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/seaportal/")
	// Run build from repo root (two levels up from this test's package dir).
	buildCmd.Dir = repoRoot(t)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	// 2) Spawn and time three cold starts.
	const runs = 3
	samples := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		d, err := measureColdStart(t, binPath)
		if err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
		samples = append(samples, d)
		t.Logf("run %d: %v", i+1, d)
	}

	// 3) Median of three.
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	median := sorted[len(sorted)/2]
	t.Logf("samples: %v  median: %v  budget: %v", samples, median, coldStartBudget)

	if median > coldStartBudget {
		t.Fatalf("MCP cold-start median %v exceeds budget %v (samples: %v)", median, coldStartBudget, samples)
	}
}

// measureColdStart spawns `<binPath> mcp`, writes a single `initialize`
// JSON-RPC request on stdin, and measures wall-clock from cmd.Start() to the
// first byte of the response line on stdout.
func measureColdStart(t *testing.T, binPath string) (time.Duration, error) {
	t.Helper()

	cmd := exec.Command(binPath, "mcp")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 0, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	// Drop stderr — we don't want it competing with our timing.
	cmd.Stderr = nil

	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		return 0, err
	}

	// Ensure the child is reaped even on error paths.
	defer func() {
		_ = stdin.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()

	// Write the initialize request.
	if _, err := stdin.Write([]byte(initializeRequest)); err != nil {
		return 0, err
	}

	// Read one byte to mark first-byte arrival, then drain the rest of the
	// line so we don't leave the child blocked on a full pipe.
	br := bufio.NewReader(stdout)
	if _, err := br.ReadByte(); err != nil {
		return 0, err
	}
	t1 := time.Now()
	// Best-effort: consume the remainder of the initialize response line.
	_, _ = br.ReadBytes('\n')

	return t1.Sub(t0), nil
}

// repoRoot walks up from the test file location to find go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not find go.mod from %s", wd)
	return ""
}
