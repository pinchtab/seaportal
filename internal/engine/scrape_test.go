package engine

import (
	"context"
	"errors"
	"testing"
)

func TestScrapeOptionsDefaults(t *testing.T) {
	got := ScrapeOptions{BaseURL: "https://example.com"}.normalized()

	if got.MaxPages != DefaultScrapeMaxPages {
		t.Errorf("MaxPages = %d, want %d", got.MaxPages, DefaultScrapeMaxPages)
	}
	if got.MaxPerPattern != DefaultScrapeMaxPerPattern {
		t.Errorf("MaxPerPattern = %d, want %d", got.MaxPerPattern, DefaultScrapeMaxPerPattern)
	}
	if got.SampleStrategy != SampleBalanced {
		t.Errorf("SampleStrategy = %q, want %q", got.SampleStrategy, SampleBalanced)
	}
	if got.Output != OutputJSON {
		t.Errorf("Output = %q, want %q", got.Output, OutputJSON)
	}
	if got.Timeout != DefaultScrapeTimeout {
		t.Errorf("Timeout = %s, want %s", got.Timeout, DefaultScrapeTimeout)
	}
	if got.RespectRobots == nil || !*got.RespectRobots {
		t.Errorf("RespectRobots = %v, want default true", got.RespectRobots)
	}
	if got.UserAgent != DefaultUserAgent {
		t.Errorf("UserAgent = %q, want %q", got.UserAgent, DefaultUserAgent)
	}
}

func TestScrapeOptionsPreservesExplicitValues(t *testing.T) {
	no := false
	got := ScrapeOptions{
		BaseURL:        "https://example.com",
		MaxPages:       5,
		MaxPerPattern:  2,
		SampleStrategy: SampleRandom,
		Output:         OutputMarkdown,
		RespectRobots:  &no,
		UserAgent:      "custom-agent",
	}.normalized()

	if got.MaxPages != 5 || got.MaxPerPattern != 2 {
		t.Errorf("caps overwritten: MaxPages=%d MaxPerPattern=%d", got.MaxPages, got.MaxPerPattern)
	}
	if got.SampleStrategy != SampleRandom || got.Output != OutputMarkdown {
		t.Errorf("strategy/output overwritten: %q %q", got.SampleStrategy, got.Output)
	}
	if got.RespectRobots == nil || *got.RespectRobots {
		t.Errorf("explicit RespectRobots=false was not preserved: %v", got.RespectRobots)
	}
	if got.UserAgent != "custom-agent" {
		t.Errorf("UserAgent overwritten: %q", got.UserAgent)
	}
}

func TestScrapeSiteNotImplemented(t *testing.T) {
	_, err := ScrapeSite(context.Background(), &ScrapeOptions{BaseURL: "https://example.com"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err = %v, want ErrNotImplemented", err)
	}
}

func TestScrapeSiteValidatesBaseURL(t *testing.T) {
	if _, err := ScrapeSite(context.Background(), &ScrapeOptions{}); !errors.Is(err, ErrMissingBaseURL) {
		t.Fatalf("empty BaseURL err = %v, want ErrMissingBaseURL", err)
	}
	if _, err := ScrapeSite(context.Background(), nil); !errors.Is(err, ErrMissingBaseURL) {
		t.Fatalf("nil opts err = %v, want ErrMissingBaseURL", err)
	}
}
