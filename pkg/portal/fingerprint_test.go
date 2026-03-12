package portal

import "testing"

func TestSemanticFingerprint(t *testing.T) {
	tests := []struct {
		name        string
		content1    string
		content2    string
		shouldMatch bool
	}{
		{
			name:        "identical content",
			content1:    "This is a test article about Go programming.",
			content2:    "This is a test article about Go programming.",
			shouldMatch: true,
		},
		{
			name:        "different timestamps same content",
			content1:    "Posted on 2024-01-15 at 10:30:00. This is an article about Go.",
			content2:    "Posted on 2024-03-10 at 14:45:00. This is an article about Go.",
			shouldMatch: true,
		},
		{
			name:        "different counters same content",
			content1:    "Views: 12847 | Likes: 523. Learn about Go concurrency.",
			content2:    "Views: 98234 | Likes: 1205. Learn about Go concurrency.",
			shouldMatch: true,
		},
		{
			name:        "relative time expressions",
			content1:    "Posted 5 minutes ago. Go is a great language.",
			content2:    "Posted 3 hours ago. Go is a great language.",
			shouldMatch: true,
		},
		{
			name:        "actually different content",
			content1:    "This article is about Go programming.",
			content2:    "This article is about Rust programming.",
			shouldMatch: false,
		},
		{
			name:        "version numbers ignored",
			content1:    "Go v1.21.0 release notes. Many improvements.",
			content2:    "Go v1.22.0 release notes. Many improvements.",
			shouldMatch: true,
		},
		{
			name:        "UUID session IDs ignored",
			content1:    "Session: 550e8400-e29b-41d4-a716-446655440000. Welcome back!",
			content2:    "Session: 7c9e6679-7425-40de-944b-e07fc1f90ae7. Welcome back!",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp1 := SemanticFingerprint(tt.content1)
			fp2 := SemanticFingerprint(tt.content2)

			if tt.shouldMatch && fp1 != fp2 {
				t.Errorf("fingerprints should match but don't:\n  fp1: %s\n  fp2: %s", fp1, fp2)
			}
			if !tt.shouldMatch && fp1 == fp2 {
				t.Errorf("fingerprints should differ but match:\n  fp1: %s", fp1)
			}
		})
	}
}

func TestChangeSignificance(t *testing.T) {
	tests := []struct {
		name     string
		old      string
		new      string
		minScore int
		maxScore int
	}{
		{
			name:     "identical content",
			old:      "This is a test article.",
			new:      "This is a test article.",
			minScore: 0,
			maxScore: 0,
		},
		{
			name:     "minor timestamp change",
			old:      "Posted 2024-01-15. Content here.",
			new:      "Posted 2024-03-10. Content here.",
			minScore: 0,
			maxScore: 10,
		},
		{
			name:     "completely different",
			old:      "Go is a programming language designed at Google.",
			new:      "Rust is a systems programming language focused on safety.",
			minScore: 50,
			maxScore: 100,
		},
		{
			name:     "partial change",
			old:      "Go is great for web services. It has excellent concurrency support.",
			new:      "Go is great for web services. It has excellent networking support.",
			minScore: 10,
			maxScore: 40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ChangeSignificance(tt.old, tt.new)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score %d not in range [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestContentChanged(t *testing.T) {
	// Same semantic content with noise differences
	old := "Last updated: 2024-01-15T10:30:00Z\nViews: 5234\nThis is the main content."
	new := "Last updated: 2024-03-10T14:45:00Z\nViews: 8921\nThis is the main content."

	if ContentChanged(old, new) {
		t.Error("ContentChanged should return false for noise-only differences")
	}

	// Actually different content
	old2 := "This article explains Go concurrency patterns."
	new2 := "This article explains Rust memory management."

	if !ContentChanged(old2, new2) {
		t.Error("ContentChanged should return true for real content differences")
	}
}
