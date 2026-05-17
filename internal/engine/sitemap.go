package engine

import (
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
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
	MaxDepth int           // default 5
	MaxURLs  int           // default 50_000
	Timeout  time.Duration // per-fetch timeout (used when Client is nil)
	Client   *http.Client  // optional; falls back to engine getClient()
}

// xml shapes
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
		opts.MaxDepth = 5
	}
	if opts.MaxURLs <= 0 {
		opts.MaxURLs = 50_000
	}
	if opts.Client == nil {
		if opts.Timeout > 0 {
			c := *getClient()
			c.Timeout = opts.Timeout
			opts.Client = &c
		} else {
			opts.Client = getClient()
		}
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

	body, err := fetchSitemap(ctx, sitemapURL, opts.Client)
	if err != nil {
		return err
	}

	// Detect root element name.
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

func fetchSitemap(ctx context.Context, sitemapURL string, client *http.Client) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/xml,text/xml,*/*;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch sitemap %s: %w", sitemapURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch sitemap %s: status %d", sitemapURL, resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	isGzip := strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") || strings.HasSuffix(strings.ToLower(sitemapURL), ".gz")
	if isGzip {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gunzip sitemap %s: %w", sitemapURL, err)
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read sitemap %s: %w", sitemapURL, err)
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
