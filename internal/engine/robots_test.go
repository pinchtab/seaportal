package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
	picked := pickRules(rules, "SomeOther/5.0 (non-matching)")
	if evaluate(picked, "/private/x") {
		t.Fatalf("non-matching UA should fall back to * rules and be blocked on /private/")
	}
	if !evaluate(picked, "/public") {
		t.Fatalf("non-matching UA should be allowed elsewhere via * fallback")
	}
}

func TestProductToken(t *testing.T) {
	cases := []struct{ ua, want string }{
		{"seaportal-test/1.0", "seaportal-test"},
		{DefaultUserAgent, "mozilla"},
		{"Googlebot", "googlebot"},
		{"  My_Bot/2.1 (+https://example.com)", "my_bot"},
		{"123nonsense", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := productToken(c.ua); got != c.want {
			t.Errorf("productToken(%q) = %q, want %q", c.ua, got, c.want)
		}
	}
}

func TestPickRules_NoSubstringMatch(t *testing.T) {
	body := "User-agent: bot\nDisallow: /\n\nUser-agent: *\nAllow: /\n"
	_, rawRules := parseRobotsTxt(body)
	rules := compileRules(rawRules)
	picked := pickRules(rules, "Mozilla/5.0 (compatible; Googlebot/2.1)")
	if !evaluate(picked, "/anything") {
		t.Fatal("section 'bot' bound a UA whose product token is 'mozilla' (substring matching regression)")
	}
}

func TestPickRules_NoUnconditionalSelfMatch(t *testing.T) {
	body := "User-agent: seaportal\nDisallow: /\n\nUser-agent: *\nAllow: /\n"
	_, rawRules := parseRobotsTxt(body)
	rules := compileRules(rawRules)
	picked := pickRules(rules, DefaultUserAgent)
	if !evaluate(picked, "/anything") {
		t.Fatal("'seaportal' section bound an impersonated Chrome UA")
	}
	picked = pickRules(rules, ResolveUserAgent("seaportal"))
	if evaluate(picked, "/anything") {
		t.Fatal("'seaportal' section should bind the seaportal persona UA")
	}
}

func TestPickRules_LongestTokenPrefixWins(t *testing.T) {
	body := "User-agent: googlebot\nDisallow: /a\n\nUser-agent: googlebot-news\nDisallow: /b\n"
	_, rawRules := parseRobotsTxt(body)
	rules := compileRules(rawRules)

	newsPicked := pickRules(rules, "Googlebot-News/1.0")
	if evaluate(newsPicked, "/b/x") || !evaluate(newsPicked, "/a/x") {
		t.Fatalf("googlebot-news should bind its own section only, got rules %+v", newsPicked)
	}
	basePicked := pickRules(rules, "Googlebot/2.1")
	if evaluate(basePicked, "/a/x") || !evaluate(basePicked, "/b/x") {
		t.Fatalf("googlebot should bind the googlebot section only, got rules %+v", basePicked)
	}
}

func TestPickDelay_TokenPrefixAndWildcard(t *testing.T) {
	delays, _ := parseRobotsTxt("User-agent: seaportal\nCrawl-delay: 7\n\nUser-agent: *\nCrawl-delay: 2\n")
	if got := pickDelay(delays, "seaportal-test/1.0"); got != 7*time.Second {
		t.Errorf("seaportal-test delay = %s, want 7s (token-prefix bind)", got)
	}
	if got := pickDelay(delays, DefaultUserAgent); got != 2*time.Second {
		t.Errorf("Chrome UA delay = %s, want 2s wildcard (no self-match leak)", got)
	}
}

func TestClampCrawlDelay(t *testing.T) {
	if d, clamped := clampCrawlDelay(86400 * time.Second); d != maxCrawlDelay || !clamped {
		t.Errorf("clampCrawlDelay(86400s) = (%s, %v), want (%s, true)", d, clamped, maxCrawlDelay)
	}
	if d, clamped := clampCrawlDelay(2 * time.Second); d != 2*time.Second || clamped {
		t.Errorf("clampCrawlDelay(2s) = (%s, %v), want (2s, false)", d, clamped)
	}
	if d, clamped := clampCrawlDelay(maxCrawlDelay); d != maxCrawlDelay || clamped {
		t.Errorf("clampCrawlDelay(max) = (%s, %v), want (%s, false)", d, clamped, maxCrawlDelay)
	}
}

func TestApplyCrawlDelay_ClampsHostileDelayWithWarning(t *testing.T) {
	srv := newRobotsServer(t, "User-agent: *\nCrawl-delay: 86400\n")
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	cache := NewCrawlDelayCache()
	if got := cache.GetDelayWithScheme(context.Background(), u.Host, testUA, "http"); got != 86400*time.Second {
		t.Fatalf("GetDelayWithScheme = %s, want the unclamped 86400s", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts := Options{RespectCrawlDelay: true, CrawlDelayCache: cache}
	warning, err := applyCrawlDelay(ctx, opts, srv.URL+"/page", u.Hostname(), testUA)
	if err == nil {
		t.Fatal("expected ctx cancellation to preempt the (clamped) wait")
	}
	if !strings.Contains(warning, "clamped") || !strings.Contains(warning, "30s") {
		t.Fatalf("warning = %q, want a clamped-to-30s notice", warning)
	}
}

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
	ctx := context.Background()

	if cache.IsAllowed(ctx, domain, testUA, "http", "/private/foo") {
		t.Fatal("expected /private/foo to be blocked")
	}
	if !cache.IsAllowed(ctx, domain, testUA, "http", "/public/foo") {
		t.Fatal("expected /public/foo to be allowed")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected single robots.txt fetch (cache hit on 2nd call); got %d", got)
	}
}

func TestIsAllowed_SingleflightAcrossGoroutines(t *testing.T) {
	var hits int32
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		<-release
		_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /private/\n")
	})
	slow := httptest.NewServer(mux)
	defer slow.Close()

	fastMux := http.NewServeMux()
	fastMux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /x/\n")
	})
	fast := httptest.NewServer(fastMux)
	defer fast.Close()

	cache := NewCrawlDelayCache()
	slowHost := strings.TrimPrefix(slow.URL, "http://")
	fastHost := strings.TrimPrefix(fast.URL, "http://")
	ctx := context.Background()

	var wg sync.WaitGroup
	blocked := make([]bool, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			blocked[i] = !cache.IsAllowed(ctx, slowHost, testUA, "http", "/private/x")
		}(i)
	}

	otherDone := make(chan bool, 1)
	go func() {
		otherDone <- cache.IsAllowed(ctx, fastHost, testUA, "http", "/x/secret")
	}()
	select {
	case allowed := <-otherDone:
		if allowed {
			t.Error("fast domain should have blocked /x/secret")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lookup for an unrelated domain blocked behind an in-flight fetch")
	}

	close(release)
	wg.Wait()
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected 1 coalesced robots.txt fetch for the slow domain, got %d", got)
	}
	for i, b := range blocked {
		if !b {
			t.Errorf("goroutine %d: expected /private/x blocked", i)
		}
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
	if !cache.IsAllowed(context.Background(), u.Host, testUA, "http", "/anything") {
		t.Fatal("expected fail-open on 404")
	}
}

func TestIsAllowed_FailsOpenOnError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cache := NewCrawlDelayCache()
	if !cache.IsAllowed(context.Background(), addr, testUA, "http", "/anything") {
		t.Fatal("expected fail-open on network error")
	}
}

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
