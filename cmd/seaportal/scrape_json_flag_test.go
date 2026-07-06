package main_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCLI_ScrapeJSONFlagAlias(t *testing.T) {
	bin := buildBinary(t)
	srv := scrapeTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "scrape", "--json", srv.URL, "--max-pages", "3").Output()
	if err != nil {
		t.Fatalf("scrape --json failed: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("scrape --json output is not valid JSON: %v\n%s", err, out)
	}
	if _, ok := res["site"]; !ok {
		t.Errorf("scrape --json output missing site object: %s", out)
	}
}

func TestCLI_ScrapeJSONConflictsWithOutput(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "scrape", "--json", "--output", "md", "https://example.com")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for --json with --output md")
	}
	if !strings.Contains(string(out), "--json conflicts with --output md") {
		t.Errorf("missing conflict message, got: %s", out)
	}
}
