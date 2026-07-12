package engine

import (
	"context"
	"testing"
)

func TestParseFeed_UnclosedCDATA_ReturnsError(t *testing.T) {
	body := loadFixture(t, "feeds/rss-unclosed-cdata.xml")
	srv := serveBody(t, "application/rss+xml", body)
	defer srv.Close()

	_, parseErr := ParseFeed(context.Background(), srv.URL, ParseFeedOptions{Client: srv.Client()})
	if parseErr == nil {
		t.Fatalf("expected error on unclosed CDATA, got nil")
	}
	t.Logf("got error: %v", parseErr)
}
