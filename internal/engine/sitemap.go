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

type SitemapEntry struct {
	Loc        string `json:"loc"`
	LastMod    string `json:"lastmod,omitempty"`
	ChangeFreq string `json:"changefreq,omitempty"`
	Priority   string `json:"priority,omitempty"`
}

type FlattenSitemapOptions struct {
	MaxDepth int
	MaxURLs  int
	Timeout  time.Duration
	Client   *http.Client
	Security *SecurityPolicy
	Since    time.Time
}

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
	if err := ctx.Err(); err != nil {
		return err
	}
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
			if err := ctx.Err(); err != nil {
				return err
			}
			loc := strings.TrimSpace(s.Loc)
			if loc == "" {
				continue
			}
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
