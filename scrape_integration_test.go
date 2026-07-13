package seaportal_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/seaportal"
	"github.com/pinchtab/seaportal/internal/testserver/fixture"
)

func pageURLs(pages []seaportal.PageObject) []string {
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.URL
	}
	sort.Strings(out)
	return out
}

func TestScrapeSiteIntegration(t *testing.T) {
	srv := fixture.MultiPageSite()
	defer srv.Close()

	sec := seaportal.DefaultSecurityPolicy()
	sec.BlockPrivateIPs = false

	opts := &seaportal.ScrapeOptions{BaseURL: srv.URL(), MaxPages: 20, MaxPerPattern: 8, Security: sec}
	res, err := seaportal.ScrapeSite(context.Background(), opts)
	if err != nil {
		t.Fatalf("ScrapeSite: %v", err)
	}

	if !res.Site.SitemapFound {
		t.Error("Site.SitemapFound = false, want true")
	}
	if res.Site.TotalURLsInSitemap != 7 {
		t.Errorf("TotalURLsInSitemap = %d, want 7", res.Site.TotalURLsInSitemap)
	}

	patterns := map[string]bool{}
	for _, g := range res.PageGroups {
		patterns[g.Pattern] = true
	}
	for _, want := range []string{"/", "/about", "/blog/*", "/products/*"} {
		if !patterns[want] {
			t.Errorf("missing pattern group %q (got %v)", want, patterns)
		}
	}
	if patterns["/private/*"] || patterns["/private/secret"] {
		t.Errorf("robots-disallowed /private grouped: %v", patterns)
	}

	if res.Site.SampledPages == 0 || res.Site.SampledPages > 20 {
		t.Errorf("SampledPages = %d, want 1..20", res.Site.SampledPages)
	}
	var haveArticle, haveProduct bool
	for _, p := range res.Pages {
		if strings.Contains(p.URL, "/private") {
			t.Errorf("robots-disallowed page present: %s", p.URL)
		}
		if p.Status != 200 || p.Error != "" {
			t.Errorf("page %s: status=%d err=%q", p.URL, p.Status, p.Error)
		}
		if len(p.Meta) == 0 || p.Markdown == "" {
			t.Errorf("page %s under-populated: meta=%v md=%q", p.URL, p.Meta, p.Markdown)
		}
		switch p.ContentType {
		case "article":
			haveArticle = true
		case "product":
			haveProduct = true
		}
	}
	if !haveArticle || !haveProduct {
		t.Errorf("contentType classification missing article/product across %d pages", len(res.Pages))
	}

	det := &seaportal.ScrapeOptions{BaseURL: srv.URL(), MaxPages: 20, MaxPerPattern: 1, Security: sec}
	a, err := seaportal.ScrapeSite(context.Background(), det)
	if err != nil {
		t.Fatalf("ScrapeSite (run a): %v", err)
	}
	b, err := seaportal.ScrapeSite(context.Background(), det)
	if err != nil {
		t.Fatalf("ScrapeSite (run b): %v", err)
	}
	ua, ub := pageURLs(a.Pages), pageURLs(b.Pages)
	if len(ua) != len(ub) {
		t.Fatalf("determinism: %d vs %d pages", len(ua), len(ub))
	}
	for i := range ua {
		if ua[i] != ub[i] {
			t.Errorf("sampling not deterministic:\n a=%v\n b=%v", ua, ub)
			break
		}
	}
}
