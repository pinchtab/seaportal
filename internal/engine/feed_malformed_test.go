package engine

import (
	"context"
	"os"
	"testing"
)

func TestParseFeed_UnclosedCDATA_ReturnsError(t *testing.T) {
	body, err := os.ReadFile("../../testdata/feeds/rss-unclosed-cdata.xml")
	if err != nil {
		t.Fatal(err)
	}
	srv := serveBody(t, "application/rss+xml", string(body))
	defer srv.Close()

	_, parseErr := ParseFeed(context.Background(), srv.URL, ParseFeedOptions{Client: srv.Client()})
	if parseErr == nil {
		t.Fatalf("expected error on unclosed CDATA, got nil")
	}
	t.Logf("got error: %v", parseErr)
}
