package main_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func scrapeTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			http.NotFound(w, r)
			return
		}
		body := `<html><head><title>` + r.URL.Path + `</title></head><body><h1>` + r.URL.Path + `</h1>`
		if r.URL.Path == "/" {
			body += `<a href="/about">About</a><a href="/blog/1">Post</a>`
		}
		body += `<p>Body content for extraction.</p></body></html>`
		w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCLI_ScrapeHelp(t *testing.T) {
	bin := buildBinary(t)
	out, _ := exec.Command(bin, "scrape", "--help").CombinedOutput()
	s := string(out)
	for _, want := range []string{"-max-pages", "-max-per-pattern", "-sample-strategy", "-output", "-with-performance", "-respect-robots", "-timeout", "-user-agent", "-full"} {
		if !strings.Contains(s, want) {
			t.Errorf("scrape --help missing flag %s\n%s", want, s)
		}
	}
}

func TestCLI_ScrapeHappyPath(t *testing.T) {
	bin := buildBinary(t)
	srv := scrapeTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "scrape", srv.URL, "--max-pages", "5").Output()
	if err != nil {
		t.Fatalf("scrape happy path failed: %v", err)
	}

	var res struct {
		Site struct {
			BaseURL string `json:"baseURL"`
		} `json:"site"`
		Pages []map[string]any `json:"pages"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if res.Site.BaseURL != srv.URL {
		t.Errorf("site.baseURL = %s, want %s", res.Site.BaseURL, srv.URL)
	}
	if len(res.Pages) == 0 {
		t.Error("expected at least one scraped page")
	}
}

func TestCLI_ScrapeInvalidFlags(t *testing.T) {
	bin := buildBinary(t)
	srv := scrapeTestServer(t)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"bad strategy", []string{"scrape", srv.URL, "--sample-strategy", "bogus"}, "sample-strategy"},
		{"bad output", []string{"scrape", srv.URL, "--output", "bogus"}, "output"},
		{"directory needs out-dir", []string{"scrape", srv.URL, "--output", "directory"}, "out-dir"},
		{"missing url", []string{"scrape"}, "base-url"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := exec.Command(bin, c.args...).CombinedOutput()
			exitErr, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("expected non-zero exit, got err=%v out=%s", err, out)
			}
			if exitErr.ExitCode() == 0 {
				t.Errorf("expected non-zero exit code")
			}
			if !strings.Contains(string(out), c.want) {
				t.Errorf("stderr should mention %q, got: %s", c.want, out)
			}
		})
	}
}
