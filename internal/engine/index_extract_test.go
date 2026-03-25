package engine

import (
	"strings"
	"testing"
)

func TestDetectIndexPage(t *testing.T) {
	tests := []struct {
		name          string
		html          string
		wantIndex     bool
		minArticles   int
		minHeadlines  int
		minConfidence int
	}{
		{
			name: "simple index page with multiple articles",
			html: `
				<html>
				<body>
					<article><h2><a href="/article1">Article One Title</a></h2><p>Teaser one</p></article>
					<article><h2><a href="/article2">Article Two Title</a></h2><p>Teaser two</p></article>
					<article><h2><a href="/article3">Article Three Title</a></h2><p>Teaser three</p></article>
					<article><h2><a href="/article4">Article Four Title</a></h2><p>Teaser four</p></article>
					<article><h2><a href="/article5">Article Five Title</a></h2><p>Teaser five</p></article>
					<article><h2><a href="/article6">Article Six Title</a></h2><p>Teaser six</p></article>
				</body>
				</html>
			`,
			wantIndex:     true,
			minArticles:   5,
			minHeadlines:  5,
			minConfidence: 50,
		},
		{
			name: "single article page",
			html: `
				<html>
				<body>
					<article>
						<h1>Main Article Title</h1>
						<p>This is the main content of the article.</p>
						<p>More content here.</p>
					</article>
				</body>
				</html>
			`,
			wantIndex:    false,
			minArticles:  0,
			minHeadlines: 0,
		},
		{
			name: "homepage with many h2 links but no article tags",
			html: `
				<html>
				<body>
					<div class="cards">
						<h2><a href="/post1">First Post</a></h2>
						<h2><a href="/post2">Second Post</a></h2>
						<h2><a href="/post3">Third Post</a></h2>
						<h2><a href="/post4">Fourth Post</a></h2>
						<h2><a href="/post5">Fifth Post</a></h2>
						<h2><a href="/post6">Sixth Post</a></h2>
						<h2><a href="/post7">Seventh Post</a></h2>
						<h2><a href="/post8">Eighth Post</a></h2>
						<h2><a href="/post9">Ninth Post</a></h2>
					</div>
				</body>
				</html>
			`,
			wantIndex:     true,
			minHeadlines:  8,
			minConfidence: 50,
		},
		{
			name: "minimal page",
			html: `
				<html>
				<body>
					<p>Hello world</p>
				</body>
				</html>
			`,
			wantIndex: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectIndexPage(tt.html)

			if result.IsIndexPage != tt.wantIndex {
				t.Errorf("IsIndexPage = %v, want %v", result.IsIndexPage, tt.wantIndex)
			}

			if result.ArticleCount < tt.minArticles {
				t.Errorf("ArticleCount = %d, want >= %d", result.ArticleCount, tt.minArticles)
			}

			if result.HeadlineCount < tt.minHeadlines {
				t.Errorf("HeadlineCount = %d, want >= %d", result.HeadlineCount, tt.minHeadlines)
			}

			if tt.wantIndex && result.Confidence < tt.minConfidence {
				t.Errorf("Confidence = %d, want >= %d", result.Confidence, tt.minConfidence)
			}
		})
	}
}

func TestExtractCardItems(t *testing.T) {
	html := `
		<html>
		<body>
			<article>
				<h2><a href="/article1">First Article</a></h2>
				<p class="excerpt">This is the excerpt</p>
			</article>
			<article>
				<h3><a href="/article2">Second Article</a></h3>
				<div class="teaser">Second teaser text</div>
			</article>
		</body>
		</html>
	`

	result := DetectIndexPage(html)

	if len(result.Items) < 2 {
		t.Fatalf("Expected at least 2 items, got %d", len(result.Items))
	}

	// Check that we extracted URLs
	hasURLs := false
	for _, item := range result.Items {
		if item.URL != "" {
			hasURLs = true
			break
		}
	}
	if !hasURLs {
		t.Error("Expected items to have URLs")
	}
}

func TestFormatIndexMarkdown(t *testing.T) {
	items := []CardItem{
		{Title: "First Article", URL: "/article1", Teaser: "First teaser"},
		{Title: "Second Article", URL: "/article2"},
	}

	md := formatIndexMarkdown(items)

	if !strings.Contains(md, "## [First Article](/article1)") {
		t.Error("Expected markdown to contain linked headline")
	}

	if !strings.Contains(md, "First teaser") {
		t.Error("Expected markdown to contain teaser")
	}

	if !strings.Contains(md, "## [Second Article](/article2)") {
		t.Error("Expected markdown to contain second headline")
	}
}

func TestShouldUseIndexFallback(t *testing.T) {
	tests := []struct {
		name                string
		readabilityLength   int
		readabilityHeadings int
		indexIsIndexPage    bool
		indexHeadlines      int
		wantFallback        bool
	}{
		{
			name:                "poor readability, rich index",
			readabilityLength:   500,
			readabilityHeadings: 1,
			indexIsIndexPage:    true,
			indexHeadlines:      20,
			wantFallback:        true,
		},
		{
			name:                "good readability, rich index",
			readabilityLength:   5000,
			readabilityHeadings: 5,
			indexIsIndexPage:    true,
			indexHeadlines:      20,
			wantFallback:        false, // readability is good enough
		},
		{
			name:                "poor readability, not index page",
			readabilityLength:   500,
			readabilityHeadings: 1,
			indexIsIndexPage:    false,
			indexHeadlines:      3,
			wantFallback:        false,
		},
		{
			name:                "poor readability, poor index",
			readabilityLength:   500,
			readabilityHeadings: 1,
			indexIsIndexPage:    true,
			indexHeadlines:      2,
			wantFallback:        false, // index not rich enough
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readResult := Result{
				Length:       tt.readabilityLength,
				HeadingCount: tt.readabilityHeadings,
			}
			indexResult := IndexPageResult{
				IsIndexPage:   tt.indexIsIndexPage,
				HeadlineCount: tt.indexHeadlines,
			}

			got := ShouldUseIndexFallback(readResult, indexResult)
			if got != tt.wantFallback {
				t.Errorf("ShouldUseIndexFallback = %v, want %v", got, tt.wantFallback)
			}
		})
	}
}
