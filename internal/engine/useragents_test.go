package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveUserAgent_KnownPreset(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"chrome", DefaultUserAgent},
		{"safari", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15"},
		{"firefox", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14.0; rv:125.0) Gecko/20100101 Firefox/125.0"},
		{"googlebot", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"},
		{"bingbot", "Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)"},
		{"seaportal", "seaportal/0.x (+https://github.com/pinchtab/seaportal)"},
		{"search-bot", "Mozilla/5.0 (compatible; SearchBot/1.0)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveUserAgent(c.name)
			if got != c.want {
				t.Errorf("ResolveUserAgent(%q) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func TestResolveUserAgent_UnknownPresetFallsThrough(t *testing.T) {
	literal := "My Custom UA/1.0"
	if got := ResolveUserAgent(literal); got != literal {
		t.Errorf("expected literal pass-through, got %q", got)
	}
}

func TestResolveUserAgent_EmptyReturnsDefault(t *testing.T) {
	if got := ResolveUserAgent(""); got != DefaultUserAgent {
		t.Errorf("expected DefaultUserAgent for empty input, got %q", got)
	}
}

func TestResolveUserAgent_CaseInsensitive(t *testing.T) {
	for _, in := range []string{"CHROME", "Chrome", "cHrOmE"} {
		if got := ResolveUserAgent(in); got != DefaultUserAgent {
			t.Errorf("ResolveUserAgent(%q) = %q, want DefaultUserAgent", in, got)
		}
	}
}

func TestExtract_UserAgentPreset(t *testing.T) {
	var capturedUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>UA Test</title></head><body><p>hello</p></body></html>`))
	}))
	defer srv.Close()

	result := FromURLWithOptions(srv.URL, Options{UserAgent: "googlebot"})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	want := "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
	if capturedUA != want {
		t.Errorf("server captured UA = %q, want %q", capturedUA, want)
	}
}

func TestExtract_DomainUserAgentWinsOverGlobal(t *testing.T) {
	var capturedUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>UA Test</title></head><body><p>hello</p></body></html>`))
	}))
	defer srv.Close()

	domain := extractDomain(srv.URL)
	perHost := "PerHost-UA/9.9"
	result := FromURLWithOptions(srv.URL, Options{
		UserAgent:       "googlebot",
		DomainUserAgent: map[string]string{domain: perHost},
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if capturedUA != perHost {
		t.Errorf("per-host UA should win: got %q, want %q", capturedUA, perHost)
	}
}
