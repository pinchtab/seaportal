package engine

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

// FeedItem is a normalised feed entry covering RSS 2.0, Atom 1.0, and
// JSON Feed 1.x sources.
type FeedItem struct {
	Title     string `json:"title,omitempty"`
	Link      string `json:"link,omitempty"`
	Published string `json:"published,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Author    string `json:"author,omitempty"`
	GUID      string `json:"guid,omitempty"`
}

// ParseFeedOptions controls ParseFeed behaviour.
type ParseFeedOptions struct {
	MaxItems int           // default 200
	Timeout  time.Duration // per-fetch timeout (used when Client is nil)
	Client   *http.Client  // optional; falls back to engine getClient()
}

// ParseFeed fetches feedURL and parses it as RSS 2.0, Atom 1.0, or JSON Feed
// 1.x, returning a unified slice of FeedItem in feed order, capped to
// opts.MaxItems.
func ParseFeed(ctx context.Context, feedURL string, opts ParseFeedOptions) ([]FeedItem, error) {
	if opts.MaxItems <= 0 {
		opts.MaxItems = 200
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

	body, err := fetchFeed(ctx, feedURL, opts.Client)
	if err != nil {
		return nil, err
	}

	items, err := parseFeedBytes(body)
	if err != nil {
		return nil, fmt.Errorf("parse feed %s: %w", feedURL, err)
	}
	if len(items) > opts.MaxItems {
		items = items[:opts.MaxItems]
	}
	return items, nil
}

func fetchFeed(ctx context.Context, feedURL string, client *http.Client) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/atom+xml,application/rss+xml,application/feed+json,application/json,application/xml,text/xml,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed %s: %w", feedURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch feed %s: status %d", feedURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read feed %s: %w", feedURL, err)
	}
	return body, nil
}

// parseFeedBytes sniffs format from the first non-whitespace byte then
// dispatches to the matching parser.
func parseFeedBytes(body []byte) ([]FeedItem, error) {
	// Skip leading whitespace + UTF-8 BOM.
	i := 0
	if len(body) >= 3 && body[0] == 0xEF && body[1] == 0xBB && body[2] == 0xBF {
		i = 3
	}
	for i < len(body) {
		b := body[i]
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			i++
			continue
		}
		break
	}
	if i >= len(body) {
		return nil, fmt.Errorf("unknown feed format: empty body")
	}

	switch body[i] {
	case '<':
		root, err := detectXMLRoot(body)
		if err != nil {
			return nil, fmt.Errorf("unknown feed format: %w", err)
		}
		switch root {
		case "rss":
			return parseRSS(body)
		case "feed":
			return parseAtom(body)
		default:
			return nil, fmt.Errorf("unknown feed format: root element %q", root)
		}
	case '{':
		return parseJSONFeed(body)
	default:
		return nil, fmt.Errorf("unknown feed format: leading byte %q", body[i])
	}
}

// ── RSS 2.0 ────────────────────────────────────────────────────────────

type rssShape struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			PubDate     string `xml:"pubDate"`
			Description string `xml:"description"`
			Author      string `xml:"author"`
			GUID        string `xml:"guid"`
		} `xml:"item"`
	} `xml:"channel"`
}

func parseRSS(body []byte) ([]FeedItem, error) {
	var doc rssShape
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse rss: %w", err)
	}
	items := make([]FeedItem, 0, len(doc.Channel.Items))
	for _, it := range doc.Channel.Items {
		items = append(items, FeedItem{
			Title:     html.UnescapeString(strings.TrimSpace(it.Title)),
			Link:      strings.TrimSpace(it.Link),
			Published: strings.TrimSpace(it.PubDate),
			Summary:   html.UnescapeString(strings.TrimSpace(it.Description)),
			Author:    strings.TrimSpace(it.Author),
			GUID:      strings.TrimSpace(it.GUID),
		})
	}
	return items, nil
}

// ── Atom 1.0 ───────────────────────────────────────────────────────────

type atomShape struct {
	XMLName xml.Name `xml:"feed"`
	Entries []struct {
		Title string `xml:"title"`
		Links []struct {
			Rel  string `xml:"rel,attr"`
			Href string `xml:"href,attr"`
		} `xml:"link"`
		Published string `xml:"published"`
		Updated   string `xml:"updated"`
		Summary   string `xml:"summary"`
		Content   string `xml:"content"`
		ID        string `xml:"id"`
		Author    struct {
			Name string `xml:"name"`
		} `xml:"author"`
	} `xml:"entry"`
}

func parseAtom(body []byte) ([]FeedItem, error) {
	var doc atomShape
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse atom: %w", err)
	}
	items := make([]FeedItem, 0, len(doc.Entries))
	for _, e := range doc.Entries {
		link := ""
		// Prefer rel="alternate" or empty rel (alternate is the spec default).
		for _, l := range e.Links {
			rel := strings.TrimSpace(l.Rel)
			if rel == "alternate" || rel == "" {
				link = strings.TrimSpace(l.Href)
				break
			}
		}
		if link == "" && len(e.Links) > 0 {
			link = strings.TrimSpace(e.Links[0].Href)
		}

		published := strings.TrimSpace(e.Published)
		if published == "" {
			published = strings.TrimSpace(e.Updated)
		}

		summary := strings.TrimSpace(e.Summary)
		if summary == "" {
			summary = strings.TrimSpace(e.Content)
		}

		items = append(items, FeedItem{
			Title:     html.UnescapeString(strings.TrimSpace(e.Title)),
			Link:      link,
			Published: published,
			Summary:   html.UnescapeString(summary),
			Author:    strings.TrimSpace(e.Author.Name),
			GUID:      strings.TrimSpace(e.ID),
		})
	}
	return items, nil
}

// ── JSON Feed 1.x ──────────────────────────────────────────────────────

type jsonFeedShape struct {
	Version string `json:"version"`
	Items   []struct {
		ID            string `json:"id"`
		URL           string `json:"url"`
		Title         string `json:"title"`
		ContentText   string `json:"content_text"`
		ContentHTML   string `json:"content_html"`
		Summary       string `json:"summary"`
		DatePublished string `json:"date_published"`
		Author        struct {
			Name string `json:"name"`
		} `json:"author"`
		Authors []struct {
			Name string `json:"name"`
		} `json:"authors"`
	} `json:"items"`
}

func parseJSONFeed(body []byte) ([]FeedItem, error) {
	var doc jsonFeedShape
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse jsonfeed: %w", err)
	}
	items := make([]FeedItem, 0, len(doc.Items))
	for _, it := range doc.Items {
		summary := strings.TrimSpace(it.Summary)
		if summary == "" {
			summary = strings.TrimSpace(it.ContentText)
		}
		author := ""
		if len(it.Authors) > 0 {
			author = strings.TrimSpace(it.Authors[0].Name)
		}
		if author == "" {
			author = strings.TrimSpace(it.Author.Name)
		}
		items = append(items, FeedItem{
			Title:     html.UnescapeString(strings.TrimSpace(it.Title)),
			Link:      strings.TrimSpace(it.URL),
			Published: strings.TrimSpace(it.DatePublished),
			Summary:   html.UnescapeString(summary),
			Author:    author,
			GUID:      strings.TrimSpace(it.ID),
		})
	}
	return items, nil
}
