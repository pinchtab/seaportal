package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SitemapEntry is a single `<url>` entry flattened from a sitemap.
type SitemapEntry struct {
	Loc        string `json:"loc"`
	LastMod    string `json:"lastmod,omitempty"`
	ChangeFreq string `json:"changefreq,omitempty"`
	Priority   string `json:"priority,omitempty"`
}

// FlattenSitemapOptions controls FlattenSitemap behaviour.
type FlattenSitemapOptions struct {
	MaxDepth int             // default 5
	MaxURLs  int             // default 50_000
	Timeout  time.Duration   // per-fetch timeout (used when Client is nil)
	Client   *http.Client    // optional; falls back to engine getClient()
	Security *SecurityPolicy // optional fetch guard (SSRF, redirects, size caps)
	// Since, when non-zero, drops `<url>` entries whose `<lastmod>` is older
	// than it, and — crucially for large news archives — skips recursing into
	// child sitemaps in an index whose own `<lastmod>` is older than it. This
	// keeps a date-partitioned archive (e.g. monthly `.gz` sitemaps going back
	// years) from being downloaded and flattened in full. Entries and index
	// children without a parseable `<lastmod>` are kept (fail-open).
	Since time.Time
}

// parseSitemapTime parses a sitemap `<lastmod>` value. Sitemaps use the W3C
// datetime profile of ISO 8601: a full timestamp (RFC 3339) or a date, with
// month- and year-only forms also seen in the wild. Returns ok=false when the
// value is empty or unparseable (callers treat that as "no date known").
func parseSitemapTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z0700",
		"2006-01-02T15:04Z0700",
		"2006-01-02",
		"2006-01",
		"2006",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// olderThanCutoff reports whether a `<lastmod>` value is strictly before the
// cutoff. A missing/unparseable date is never "older" — we keep it rather than
// silently drop content whose age we cannot determine.
func olderThanCutoff(lastmod string, cutoff time.Time) bool {
	if cutoff.IsZero() {
		return false
	}
	t, ok := parseSitemapTime(lastmod)
	if !ok {
		return false
	}
	return t.Before(cutoff)
}

type sitemapURLNode struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod"`
	ChangeFreq string `xml:"changefreq"`
	Priority   string `xml:"priority"`
}

type urlsetDoc struct {
	XMLName xml.Name         `xml:"urlset"`
	URLs    []sitemapURLNode `xml:"url"`
}

type sitemapIndexEntry struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod"`
}

type sitemapIndexDoc struct {
	XMLName  xml.Name            `xml:"sitemapindex"`
	Sitemaps []sitemapIndexEntry `xml:"sitemap"`
}

// FlattenSitemap fetches sitemapURL and recursively flattens sitemap-index
// references into a single slice of SitemapEntry. Bounded by opts.MaxDepth and
// opts.MaxURLs. Deduplicated by Loc. Handles `.gz` URLs and `Content-Encoding: gzip`.
func FlattenSitemap(ctx context.Context, sitemapURL string, opts FlattenSitemapOptions) ([]SitemapEntry, error) {
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = DefaultSitemapMaxDepth
	}
	if opts.MaxURLs <= 0 {
		opts.MaxURLs = DefaultSitemapMaxURLs
	}
	visited := map[string]bool{}
	seen := map[string]bool{}
	var entries []SitemapEntry

	err := flattenSitemap(ctx, sitemapURL, 0, opts, visited, seen, &entries)
	if err != nil {
		return entries, err
	}
	return entries, nil
}

func flattenSitemap(ctx context.Context, sitemapURL string, depth int, opts FlattenSitemapOptions, visited, seen map[string]bool, entries *[]SitemapEntry) error {
	if depth > opts.MaxDepth {
		return nil
	}
	if len(*entries) >= opts.MaxURLs {
		return nil
	}
	if visited[sitemapURL] {
		return nil
	}
	visited[sitemapURL] = true

	body, err := fetchSitemap(ctx, sitemapURL, opts)
	if err != nil {
		return err
	}

	rootName, err := detectXMLRoot(body)
	if err != nil {
		return fmt.Errorf("parse sitemap %s: %w", sitemapURL, err)
	}

	switch rootName {
	case "sitemapindex":
		var doc sitemapIndexDoc
		if err := xml.Unmarshal(body, &doc); err != nil {
			return fmt.Errorf("parse sitemapindex %s: %w", sitemapURL, err)
		}
		for _, s := range doc.Sitemaps {
			loc := strings.TrimSpace(s.Loc)
			if loc == "" {
				continue
			}
			// Skip whole child sitemaps that predate the cutoff — this is what
			// makes a years-deep monthly archive index cheap to walk.
			if olderThanCutoff(s.LastMod, opts.Since) {
				continue
			}
			if len(*entries) >= opts.MaxURLs {
				return nil
			}
			if err := flattenSitemap(ctx, loc, depth+1, opts, visited, seen, entries); err != nil {
				return err
			}
		}
	case "urlset":
		var doc urlsetDoc
		if err := xml.Unmarshal(body, &doc); err != nil {
			return fmt.Errorf("parse urlset %s: %w", sitemapURL, err)
		}
		for _, u := range doc.URLs {
			loc := strings.TrimSpace(u.Loc)
			if loc == "" {
				continue
			}
			if seen[loc] {
				continue
			}
			if olderThanCutoff(u.LastMod, opts.Since) {
				continue
			}
			seen[loc] = true
			*entries = append(*entries, SitemapEntry{
				Loc:        loc,
				LastMod:    strings.TrimSpace(u.LastMod),
				ChangeFreq: strings.TrimSpace(u.ChangeFreq),
				Priority:   strings.TrimSpace(u.Priority),
			})
			if len(*entries) >= opts.MaxURLs {
				return nil
			}
		}
	default:
		return fmt.Errorf("unrecognised sitemap root element %q at %s", rootName, sitemapURL)
	}
	return nil
}

func fetchSitemap(ctx context.Context, sitemapURL string, opts FlattenSitemapOptions) ([]byte, error) {
	body, _, status, err := FetchBytes(ctx, sitemapURL, FetchBytesOptions{
		Client:   opts.Client,
		Timeout:  opts.Timeout,
		Security: opts.Security,
		Accept:   "application/xml,text/xml,*/*;q=0.8",
	})
	if err != nil {
		return nil, fmt.Errorf("fetch sitemap %s: %w", sitemapURL, err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("fetch sitemap %s: status %d", sitemapURL, status)
	}
	if strings.HasSuffix(strings.ToLower(sitemapURL), ".gz") {
		gz, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("gunzip sitemap %s: %w", sitemapURL, err)
		}
		defer func() { _ = gz.Close() }()
		// Cap the gunzip output with the policy's decompression budget: a
		// tiny .gz sitemap can otherwise expand into a decompression bomb
		// even when MaxResponseBytes capped the wire bytes (T01).
		var maxDecomp int64
		if opts.Security != nil {
			maxDecomp = opts.Security.MaxDecompressedBytes
		}
		body, err = limitedReadAll(gz, maxDecomp, ErrDecompressTooLarge)
		if err != nil {
			return nil, fmt.Errorf("read sitemap %s: %w", sitemapURL, err)
		}
	}
	return body, nil
}

// detectXMLRoot returns the local name of the first XML start element.
func detectXMLRoot(data []byte) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local, nil
		}
	}
}
