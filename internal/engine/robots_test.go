package engine

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

const testUA = "seaportal-test/1.0"

func mustAllowed(t *testing.T, content, path string, want bool) {
	t.Helper()
	_, rawRules := parseRobotsTxt(content)
	rules := compileRules(rawRules)
	picked := pickRules(rules, testUA)
	got := evaluate(picked, path)
	if got != want {
		t.Fatalf("path %q: got allowed=%v want %v (rules=%+v)", path, got, want, picked)
	}
}

// evaluate mirrors IsAllowed's matching loop for unit-level parser testing,
// avoiding the network/cache path.
func evaluate(rules []compiledRule, path string) bool {
	bestLen := -1
	bestAllow := true
	for _, r := range rules {
		if !ruleMatches(r, path) {
			continue
		}
		l := len(r.Pattern)
		switch {
		case l > bestLen:
			bestLen = l
			bestAllow = r.Allow
		case l == bestLen && r.Allow:
			bestAllow = true
		}
	}
	if bestLen < 0 {
		return true
	}
	return bestAllow
}

func TestParseRobots_AllowDisallowBasic(t *testing.T) {
	body := "User-agent: *\nDisallow: /private/\nAllow: /public/\n"
	mustAllowed(t, body, "/private/secret", false)
	mustAllowed(t, body, "/public/page", true)
	mustAllowed(t, body, "/other", true)
}

func TestParseRobots_LongestMatchWins(t *testing.T) {
	body := "User-agent: *\nDisallow: /\nAllow: /api/\n"
	mustAllowed(t, body, "/api/foo", true)
	mustAllowed(t, body, "/home", false)
}

func TestParseRobots_AllowBeatsDisallowOnTie(t *testing.T) {
	body := "User-agent: *\nDisallow: /foo\nAllow: /foo\n"
	mustAllowed(t, body, "/foo/bar", true)
}

func TestParseRobots_WildcardStar(t *testing.T) {
	body := "User-agent: *\nDisallow: /*.pdf$\n"
	mustAllowed(t, body, "/file.pdf", false)
	mustAllowed(t, body, "/deep/path/report.pdf", false)
	mustAllowed(t, body, "/file.pdf.html", true)
	mustAllowed(t, body, "/file.txt", true)
}

func TestParseRobots_EndAnchorDollar(t *testing.T) {
	body := "User-agent: *\nDisallow: /private/$\n"
	mustAllowed(t, body, "/private/", false)
	mustAllowed(t, body, "/private/x", true)
}

func TestParseRobots_UserAgentSpecificity(t *testing.T) {
	body := "User-agent: *\nDisallow: /\n\nUser-agent: seaportal-test\nAllow: /\n"
	_, rawRules := parseRobotsTxt(body)
	rules := compileRules(rawRules)
	picked := pickRules(rules, testUA)
	if !evaluate(picked, "/anything") {
		t.Fatalf("expected UA-specific Allow to win over wildcard Disallow")
	}
}

func TestParseRobots_WildcardFallback(t *testing.T) {
	body := "User-agent: googlebot\nAllow: /\n\nUser-agent: *\nDisallow: /private/\n"
	_, rawRules := parseRobotsTxt(body)
	rules := compileRules(rawRules)
	picked := pickRules(rules, "Mozilla/5.0 (non-matching)")
	if evaluate(picked, "/private/x") {
		t.Fatalf("non-matching UA should fall back to * rules and be blocked on /private/")
	}
	if !evaluate(picked, "/public") {
		t.Fatalf("non-matching UA should be allowed elsewhere via * fallback")
	}
}

// ---- IsAllowed cache (integration with httptest) ----

func TestIsAllowed_FetchesAndCaches(t *testing.T) {
	var hits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /private/\n")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cache := NewCrawlDelayCache()
	u, _ := url.Parse(srv.URL)
	domain := u.Host

	if cache.IsAllowed(domain, testUA, "http", "/private/foo") {
		t.Fatal("expected /private/foo to be blocked")
	}
	if !cache.IsAllowed(domain, testUA, "http", "/public/foo") {
		t.Fatal("expected /public/foo to be allowed")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected single robots.txt fetch (cache hit on 2nd call); got %d", got)
	}
}

func TestIsAllowed_FailsOpenOn404(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cache := NewCrawlDelayCache()
	u, _ := url.Parse(srv.URL)
	if !cache.IsAllowed(u.Host, testUA, "http", "/anything") {
		t.Fatal("expected fail-open on 404")
	}
}

func TestIsAllowed_FailsOpenOnError(t *testing.T) {
	// Bind a port then close it so the dial fails immediately.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cache := NewCrawlDelayCache()
	if !cache.IsAllowed(addr, testUA, "http", "/anything") {
		t.Fatal("expected fail-open on network error")
	}
}

// ---- Integration: FromURLWithOptions honours RespectRobots ----

func newRobotsServer(t *testing.T, robotsBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, robotsBody)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!doctype html><html><head><title>OK</title></head><body><h1>Hello</h1><p>Body content for `+r.URL.Path+` with enough text to register as real content. Lorem ipsum dolor sit amet, consectetur adipiscing elit.</p></body></html>`)
	})
	return httptest.NewServer(mux)
}

func TestExtract_RespectRobotsBlocksDisallowedPath(t *testing.T) {
	srv := newRobotsServer(t, "User-agent: *\nDisallow: /private/\n")
	defer srv.Close()

	opts := Options{RespectRobots: true, CrawlDelayCache: NewCrawlDelayCache()}
	res := FromURLWithOptions(srv.URL+"/private/foo", opts)
	if !res.BlockedByRobots {
		t.Fatalf("expected BlockedByRobots=true; got result=%+v", res)
	}
	if res.Error != "blocked by robots.txt" {
		t.Fatalf("expected Error=\"blocked by robots.txt\"; got %q", res.Error)
	}
	found := false
	for _, r := range res.Profile.Reasons {
		if r == "blocked-by-robots" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Profile.Reasons to contain \"blocked-by-robots\"; got %v", res.Profile.Reasons)
	}
}

func TestExtract_RespectRobotsAllowsAllowedPath(t *testing.T) {
	srv := newRobotsServer(t, "User-agent: *\nDisallow: /private/\n")
	defer srv.Close()

	opts := Options{RespectRobots: true, CrawlDelayCache: NewCrawlDelayCache()}
	res := FromURLWithOptions(srv.URL+"/public/foo", opts)
	if res.BlockedByRobots {
		t.Fatalf("did not expect BlockedByRobots for /public/foo; got %+v", res)
	}
	if res.Error != "" {
		t.Fatalf("did not expect Error; got %q", res.Error)
	}
	if !strings.Contains(res.Content, "Body content") && res.Title == "" {
		t.Fatalf("expected real extraction; got content=%q title=%q", res.Content, res.Title)
	}
}

func TestExtract_RespectRobotsOffByDefault(t *testing.T) {
	srv := newRobotsServer(t, "User-agent: *\nDisallow: /private/\n")
	defer srv.Close()

	res := FromURLWithOptions(srv.URL+"/private/foo", Options{})
	if res.BlockedByRobots {
		t.Fatalf("expected no robots gate when RespectRobots is false; got %+v", res)
	}
	if res.Error != "" {
		t.Fatalf("did not expect Error; got %q", res.Error)
	}
}
