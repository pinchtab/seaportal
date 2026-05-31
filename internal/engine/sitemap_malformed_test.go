package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFlattenSitemap_MalformedXML_ReturnsError(t *testing.T) {
	body, err := os.ReadFile("../../testdata/sitemaps/malformed-truncated.xml")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	_, parseErr := FlattenSitemap(context.Background(), srv.URL+"/sitemap.xml", FlattenSitemapOptions{Client: srv.Client()})
	if parseErr == nil {
		t.Fatalf("expected error on truncated sitemap, got nil")
	}
	t.Logf("got error: %v", parseErr)
}
