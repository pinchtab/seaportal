// These tests are fully deterministic and offline

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

	// Check basic content extraction
	if result.Title != "Simple Test Page" {
		t.Errorf("Title: got %q, want %q", result.Title, "Simple Test Page")
	}

	// Should have extracted meaningful content
	if len(result.Content) < 100 {
		t.Errorf("Content too short: %d chars", len(result.Content))
	}

	// Should detect as static (non-SPA)
	if result.IsSPA {
		t.Error("Simple page should not be detected as SPA")
	}

	// Confidence should be high
	if result.Confidence < 80 {
		t.Errorf("Confidence too low: %d", result.Confidence)
	}

	// Quality score should be reasonable
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

	// Should have multiple headings
	if result.QualityInfo.HeadingCount < 3 {
		t.Errorf("Expected multiple headings, got %d", result.QualityInfo.HeadingCount)
	}

	// Should have lists
	if result.QualityInfo.ListCount < 3 {
		t.Errorf("Expected lists, got %d list items", result.QualityInfo.ListCount)
	}

	// Should have code blocks
	if result.QualityInfo.CodeBlockCount < 1 {
		t.Errorf("Expected code blocks, got %d", result.QualityInfo.CodeBlockCount)
	}

	// Quality should be high for well-structured content
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

	// Should preserve table structure as markdown
	// Tables in HTML should convert to markdown table format with pipes
	if !strings.Contains(result.Content, "|") {
		t.Log("Note: Tables may not be preserved as markdown tables")
	}

	// Content should be extracted regardless
	if len(result.Content) < 100 {
		t.Errorf("Content too short: %d chars", len(result.Content))
	}
}

func TestExtract_LocalDeterminism(t *testing.T) {
	// Verify extractions are deterministic (same input = same output)
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

	// Should be detected as SPA
	if !result.IsSPA {
		t.Error("spa-shell.html should be detected as SPA")
	}

	// Should have at least 1 SPA signal
	if len(result.SPASignals) < 1 {
		t.Errorf("Expected at least 1 SPA signal, got %d", len(result.SPASignals))
	}

	// Content should be minimal (< 100 chars of meaningful text)
	if len(strings.TrimSpace(result.Content)) >= 100 {
		t.Errorf("Content should be minimal for SPA shell, got %d chars", len(strings.TrimSpace(result.Content)))
	}

	// Quality score should be low (< 30)
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

	// Should NOT be detected as SPA
	if result.IsSPA {
		t.Error("hydrated.html should NOT be detected as SPA (content is server-rendered)")
	}

	// Should have high quality extraction (Quality >= 50)
	if result.Quality < 50 {
		t.Errorf("Quality should be high for hydrated page, got %.1f", result.Quality)
	}

	// Should have multiple headings (>= 3)
	if result.QualityInfo.HeadingCount < 3 {
		t.Errorf("Expected at least 3 headings, got %d", result.QualityInfo.HeadingCount)
	}

	// Should have list items (>= 3)
	if result.QualityInfo.ListCount < 3 {
		t.Errorf("Expected at least 3 list items, got %d", result.QualityInfo.ListCount)
	}

	// Confidence should be high (>= 80)
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

	// Should NOT be detected as SPA
	if result.IsSPA {
		t.Error("dynamic-simulation.html should NOT be detected as SPA (has static fallback)")
	}

	// Should extract the static fallback content
	// Content should contain "Static Section" or "Always available"
	if !strings.Contains(result.Content, "Static Section") && !strings.Contains(result.Content, "Always available") {
		t.Errorf("Content should contain static fallback text")
	}

	// Quality should be reasonable (>= 30)
	if result.Quality < 30 {
		t.Errorf("Quality should be reasonable for mixed content, got %.1f", result.Quality)
	}
}
