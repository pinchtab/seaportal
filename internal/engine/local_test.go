package engine

import (
	"strings"
	"testing"

	"github.com/pinchtab/seaportal/internal/testserver"
)

func TestExtract_LocalSimplePage(t *testing.T) {
	srv := testserver.Start(0)
	defer srv.Stop()

	url := srv.URL() + "/static/simple.html"
	result := FromURL(url)

	if result.Error != "" {
		t.Fatalf("Extraction failed: %s", result.Error)
	}

	if result.Title != "Simple Test Page" {
		t.Errorf("Title: got %q, want %q", result.Title, "Simple Test Page")
	}

	if len(result.Content) < 100 {
		t.Errorf("Content too short: %d chars", len(result.Content))
	}

	if result.IsSPA {
		t.Error("Simple page should not be detected as SPA")
	}

	if result.Confidence < 80 {
		t.Errorf("Confidence too low: %d", result.Confidence)
	}

	if result.Quality < 30 {
		t.Errorf("Quality score too low: %.1f", result.Quality)
	}
}

func TestExtract_LocalArticle(t *testing.T) {
	srv := testserver.Start(0)
	defer srv.Stop()

	url := srv.URL() + "/static/article.html"
	result := FromURL(url)

	if result.Error != "" {
		t.Fatalf("Extraction failed: %s", result.Error)
	}

	if result.QualityInfo.HeadingCount < 3 {
		t.Errorf("Expected multiple headings, got %d", result.QualityInfo.HeadingCount)
	}

	if result.QualityInfo.ListCount < 3 {
		t.Errorf("Expected lists, got %d list items", result.QualityInfo.ListCount)
	}

	if result.QualityInfo.CodeBlockCount < 1 {
		t.Errorf("Expected code blocks, got %d", result.QualityInfo.CodeBlockCount)
	}

	if result.Quality < 50 {
		t.Errorf("Quality too low for rich article: %.1f", result.Quality)
	}
}

func TestExtract_LocalTable(t *testing.T) {
	srv := testserver.Start(0)
	defer srv.Stop()

	url := srv.URL() + "/static/table.html"
	result := FromURL(url)

	if result.Error != "" {
		t.Fatalf("Extraction failed: %s", result.Error)
	}

	if !strings.Contains(result.Content, "|") {
		t.Log("Note: Tables may not be preserved as markdown tables")
	}

	if len(result.Content) < 100 {
		t.Errorf("Content too short: %d chars", len(result.Content))
	}
}

func TestExtract_LocalDeterminism(t *testing.T) {
	srv := testserver.Start(0)
	defer srv.Stop()

	url := srv.URL() + "/static/simple.html"

	result1 := FromURL(url)
	result2 := FromURL(url)

	if result1.Content != result2.Content {
		t.Error("Extraction not deterministic: content differs")
	}

	if result1.Title != result2.Title {
		t.Error("Extraction not deterministic: title differs")
	}

	if result1.Quality != result2.Quality {
		t.Error("Extraction not deterministic: quality differs")
	}
}

func BenchmarkExtract_Local(b *testing.B) {
	srv := testserver.Start(0)
	defer srv.Stop()

	url := srv.URL() + "/static/simple.html"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FromURL(url)
	}
}

func TestExtract_LocalSPAShell(t *testing.T) {
	srv := testserver.Start(0)
	defer srv.Stop()

	url := srv.URL() + "/static/spa-shell.html"
	result := FromURL(url)

	if result.Error != "" {
		t.Fatalf("Extraction failed: %s", result.Error)
	}

	if !result.IsSPA {
		t.Error("spa-shell.html should be detected as SPA")
	}

	if len(result.SPASignals) < 1 {
		t.Errorf("Expected at least 1 SPA signal, got %d", len(result.SPASignals))
	}

	if len(strings.TrimSpace(result.Content)) >= 100 {
		t.Errorf("Content should be minimal for SPA shell, got %d chars", len(strings.TrimSpace(result.Content)))
	}

	if result.Quality >= 30 {
		t.Errorf("Quality score should be low for SPA shell, got %.1f", result.Quality)
	}
}

func TestExtract_LocalHydrated(t *testing.T) {
	srv := testserver.Start(0)
	defer srv.Stop()

	url := srv.URL() + "/static/hydrated.html"
	result := FromURL(url)

	if result.Error != "" {
		t.Fatalf("Extraction failed: %s", result.Error)
	}

	if result.IsSPA {
		t.Error("hydrated.html should NOT be detected as SPA (content is server-rendered)")
	}

	if result.Quality < 50 {
		t.Errorf("Quality should be high for hydrated page, got %.1f", result.Quality)
	}

	if result.QualityInfo.HeadingCount < 3 {
		t.Errorf("Expected at least 3 headings, got %d", result.QualityInfo.HeadingCount)
	}

	if result.QualityInfo.ListCount < 3 {
		t.Errorf("Expected at least 3 list items, got %d", result.QualityInfo.ListCount)
	}

	if result.Confidence < 80 {
		t.Errorf("Confidence should be high, got %d", result.Confidence)
	}
}

func TestExtract_LocalDynamicSimulation(t *testing.T) {
	srv := testserver.Start(0)
	defer srv.Stop()

	url := srv.URL() + "/static/dynamic-simulation.html"
	result := FromURL(url)

	if result.Error != "" {
		t.Fatalf("Extraction failed: %s", result.Error)
	}

	if result.IsSPA {
		t.Error("dynamic-simulation.html should NOT be detected as SPA (has static fallback)")
	}

	if !strings.Contains(result.Content, "Static Section") && !strings.Contains(result.Content, "Always available") {
		t.Errorf("Content should contain static fallback text")
	}

	if result.Quality < 30 {
		t.Errorf("Quality should be reasonable for mixed content, got %.1f", result.Quality)
	}
}
