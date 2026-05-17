package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func serveBody(t *testing.T, contentType, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		_, _ = w.Write([]byte(body))
	}))
	return srv
}

func TestParseFeed_RSS20(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example</title>
    <item>
      <title>First &amp; Friends</title>
      <link>https://example.com/1</link>
      <pubDate>Mon, 01 Jan 2025 10:00:00 GMT</pubDate>
      <description>Summary one &lt;b&gt;bold&lt;/b&gt;</description>
      <author>alice@example.com</author>
      <guid>guid-1</guid>
    </item>
    <item>
      <title>Second</title>
      <link>https://example.com/2</link>
      <pubDate>Tue, 02 Jan 2025 10:00:00 GMT</pubDate>
      <description>Summary two</description>
      <author>bob@example.com</author>
      <guid>guid-2</guid>
    </item>
    <item>
      <title>Third</title>
      <link>https://example.com/3</link>
      <pubDate>Wed, 03 Jan 2025 10:00:00 GMT</pubDate>
      <description>Summary three</description>
      <author>carol@example.com</author>
      <guid>guid-3</guid>
    </item>
  </channel>
</rss>`
	srv := serveBody(t, "application/rss+xml", body)
	defer srv.Close()

	items, err := ParseFeed(context.Background(), srv.URL, ParseFeedOptions{Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3, got %d", len(items))
	}
	if items[0].Title != "First & Friends" {
		t.Errorf("title not HTML-unescaped: %q", items[0].Title)
	}
	if !strings.Contains(items[0].Summary, "<b>bold</b>") {
		t.Errorf("summary not HTML-unescaped: %q", items[0].Summary)
	}
	if items[0].Link != "https://example.com/1" || items[0].GUID != "guid-1" ||
		items[0].Author != "alice@example.com" || items[0].Published == "" {
		t.Errorf("unexpected item[0]: %+v", items[0])
	}
}

func TestParseFeed_Atom10(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example</title>
  <entry>
    <title>Entry One</title>
    <link href="https://example.com/a" />
    <id>tag:example,2025:a</id>
    <published>2025-01-01T10:00:00Z</published>
    <updated>2025-01-02T10:00:00Z</updated>
    <summary>Atom summary one</summary>
    <author><name>Alice</name></author>
  </entry>
  <entry>
    <title>Entry Two</title>
    <link href="https://example.com/b" />
    <id>tag:example,2025:b</id>
    <updated>2025-02-02T10:00:00Z</updated>
    <summary>Atom summary two</summary>
    <author><name>Bob</name></author>
  </entry>
</feed>`
	srv := serveBody(t, "application/atom+xml", body)
	defer srv.Close()

	items, err := ParseFeed(context.Background(), srv.URL, ParseFeedOptions{Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2, got %d", len(items))
	}
	if items[0].Title != "Entry One" || items[0].Link != "https://example.com/a" {
		t.Errorf("entry[0] = %+v", items[0])
	}
	if items[0].Published != "2025-01-01T10:00:00Z" {
		t.Errorf("entry[0].Published = %q (want published, not updated)", items[0].Published)
	}
	if items[1].Published != "2025-02-02T10:00:00Z" {
		t.Errorf("entry[1].Published = %q (want fallback to updated)", items[1].Published)
	}
	if items[0].Author != "Alice" || items[0].GUID != "tag:example,2025:a" {
		t.Errorf("entry[0] author/id = %+v", items[0])
	}
}

func TestParseFeed_JSONFeed1(t *testing.T) {
	body := `{
  "version": "https://jsonfeed.org/version/1.1",
  "title": "Example",
  "items": [
    {
      "id": "id-1",
      "url": "https://example.com/p1",
      "title": "Post &amp; One",
      "summary": "JSON summary one",
      "date_published": "2025-01-01T10:00:00Z",
      "authors": [{"name": "Alice"}, {"name": "Bob"}]
    },
    {
      "id": "id-2",
      "url": "https://example.com/p2",
      "title": "Post Two",
      "content_text": "Body text used as fallback",
      "date_published": "2025-01-02T10:00:00Z",
      "author": {"name": "Carol"}
    }
  ]
}`
	srv := serveBody(t, "application/feed+json", body)
	defer srv.Close()

	items, err := ParseFeed(context.Background(), srv.URL, ParseFeedOptions{Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2, got %d", len(items))
	}
	if items[0].Title != "Post & One" {
		t.Errorf("title not unescaped: %q", items[0].Title)
	}
	if items[0].Link != "https://example.com/p1" || items[0].GUID != "id-1" {
		t.Errorf("item[0] = %+v", items[0])
	}
	if items[0].Author != "Alice" {
		t.Errorf("item[0].Author = %q (want Authors[0])", items[0].Author)
	}
	if items[1].Summary != "Body text used as fallback" {
		t.Errorf("item[1].Summary = %q (want content_text fallback)", items[1].Summary)
	}
	if items[1].Author != "Carol" {
		t.Errorf("item[1].Author = %q (want Author.Name fallback)", items[1].Author)
	}
}

func TestParseFeed_UnknownFormatErrors(t *testing.T) {
	srv := serveBody(t, "text/plain", "this is just random text, not a feed at all")
	defer srv.Close()

	_, err := ParseFeed(context.Background(), srv.URL, ParseFeedOptions{Client: srv.Client()})
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error should mention 'unknown': %v", err)
	}
}

func TestParseFeed_HonorsMaxItems(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel>`)
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&sb, `<item><title>t%d</title><link>https://example.com/%d</link></item>`, i, i)
	}
	sb.WriteString(`</channel></rss>`)

	srv := serveBody(t, "application/rss+xml", sb.String())
	defer srv.Close()

	items, err := ParseFeed(context.Background(), srv.URL, ParseFeedOptions{MaxItems: 10, Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(items) != 10 {
		t.Fatalf("want 10, got %d", len(items))
	}
	if items[0].Title != "t0" || items[9].Title != "t9" {
		t.Errorf("order broken: first=%q last=%q", items[0].Title, items[9].Title)
	}
}

func TestParseFeed_AtomMultipleLinks(t *testing.T) {
	body := `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <title>Mixed Links</title>
    <link rel="self" href="https://example.com/self" />
    <link rel="alternate" href="https://example.com/alternate" />
    <link rel="edit" href="https://example.com/edit" />
    <id>x</id>
  </entry>
</feed>`
	srv := serveBody(t, "application/atom+xml", body)
	defer srv.Close()

	items, err := ParseFeed(context.Background(), srv.URL, ParseFeedOptions{Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1, got %d", len(items))
	}
	if items[0].Link != "https://example.com/alternate" {
		t.Errorf("Link = %q (want alternate)", items[0].Link)
	}
}

func TestParseFeed_AtomContentFallback(t *testing.T) {
	body := `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <title>Content Only</title>
    <link href="https://example.com/c" />
    <id>c</id>
    <content type="html">Here is the content body</content>
  </entry>
</feed>`
	srv := serveBody(t, "application/atom+xml", body)
	defer srv.Close()

	items, err := ParseFeed(context.Background(), srv.URL, ParseFeedOptions{Client: srv.Client()})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1, got %d", len(items))
	}
	if items[0].Summary != "Here is the content body" {
		t.Errorf("Summary = %q (want content fallback)", items[0].Summary)
	}
}

func TestParseFeed_HTTPRoundTrip(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/rss", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><item><title>R</title><link>https://example.com/r</link></item></channel></rss>`))
	})
	mux.HandleFunc("/atom", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><title>A</title><link href="https://example.com/a"/><id>a</id></entry></feed>`))
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/feed+json")
		_, _ = w.Write([]byte(`{"version":"https://jsonfeed.org/version/1.1","items":[{"id":"j","url":"https://example.com/j","title":"J"}]}`))
	})

	for _, c := range []struct {
		path  string
		title string
		link  string
	}{
		{"/rss", "R", "https://example.com/r"},
		{"/atom", "A", "https://example.com/a"},
		{"/json", "J", "https://example.com/j"},
	} {
		items, err := ParseFeed(context.Background(), srv.URL+c.path, ParseFeedOptions{Client: srv.Client()})
		if err != nil {
			t.Fatalf("%s: unexpected err: %v", c.path, err)
		}
		if len(items) != 1 || items[0].Title != c.title || items[0].Link != c.link {
			t.Errorf("%s: got %+v want title=%q link=%q", c.path, items, c.title, c.link)
		}
	}
}

func TestCLI_FeedSubcommand(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/feed.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel>
<item><title>One</title><link>https://example.com/cli-1</link><pubDate>2025-01-01</pubDate></item>
<item><title>Two</title><link>https://example.com/cli-2</link><pubDate>2025-01-02</pubDate></item>
</channel></rss>`))
	})

	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	bin := filepath.Join(t.TempDir(), "seaportal-feed-test")

	build := exec.Command("go", "build", "-o", bin, "./cmd/seaportal")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	defer func() { _ = os.Remove(bin) }()

	// TSV mode
	cmd := exec.Command(bin, "feed", srv.URL+"/feed.xml")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI failed: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	wantLines := []string{
		"2025-01-01\tOne\thttps://example.com/cli-1",
		"2025-01-02\tTwo\thttps://example.com/cli-2",
	}
	want := strings.Join(wantLines, "\n")
	if got != want {
		t.Errorf("CLI TSV mismatch:\nwant:\n%s\ngot:\n%s", want, got)
	}

	// JSON mode
	cmd = exec.Command(bin, "feed", "--json", srv.URL+"/feed.xml")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI --json failed: %v\n%s", err, out)
	}
	var items []FeedItem
	if err := json.Unmarshal(out, &items); err != nil {
		t.Fatalf("CLI --json output not valid JSON: %v\nout=%s", err, out)
	}
	if len(items) != 2 || items[0].Title != "One" || items[1].Link != "https://example.com/cli-2" {
		t.Errorf("CLI --json items unexpected: %+v", items)
	}
}
