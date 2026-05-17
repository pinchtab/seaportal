package engine

import (
	"testing"
	"time"

	"github.com/pinchtab/seaportal/internal/engine/leakcheck"
	"github.com/pinchtab/seaportal/internal/testserver/fixture"
)

// regression: robots-crawl-delay-timing
//
// Locks the wall-clock semantics of crawl-delay enforcement end-to-end.
// `extract.go` reads the parsed URL's host[:port] (not bare hostname) and
// threads it through `CrawlDelayCache`, so non-default-port targets get
// real delay enforcement. This test fires two requests against an httptest
// server publishing `Crawl-delay: 1` and asserts the gap is within
// [0.9s, 1.4s] — the parsed delay with generous CI jitter tolerance.
func TestRobotsCrawlDelay_GapBetweenRequests(t *testing.T) {
	leakcheck.CheckLeak(t)
	const html = "<!doctype html><html><head><title>ok</title></head><body>" +
		"<p>Body content with enough words to register as real content. " +
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit.</p></body></html>"

	srv := fixture.New().
		Route("GET", "/robots.txt", fixture.Body(
			[]byte("User-agent: *\nCrawl-delay: 1\n"),
			"text/plain",
		)).
		Route("GET", "/page1", fixture.Body([]byte(html), "text/html; charset=utf-8")).
		Route("GET", "/page2", fixture.Body([]byte(html), "text/html; charset=utf-8"))
	defer srv.Close()

	opts := Options{
		RespectCrawlDelay: true,
		CrawlDelayCache:   NewCrawlDelayCache(),
	}

	res1 := FromURLWithOptions(srv.URL()+"/page1", opts)
	t1 := time.Now()
	if res1.Error != "" {
		t.Fatalf("first request errored: %q", res1.Error)
	}

	res2 := FromURLWithOptions(srv.URL()+"/page2", opts)
	t2 := time.Now()
	if res2.Error != "" {
		t.Fatalf("second request errored: %q", res2.Error)
	}

	gap := t2.Sub(t1)
	const (
		lower = 900 * time.Millisecond
		upper = 1400 * time.Millisecond
	)
	if gap < lower || gap > upper {
		t.Fatalf("gap = %v; want within [%v, %v]", gap, lower, upper)
	}
}
