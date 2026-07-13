package engine

import (
	"reflect"
	"strings"
	"testing"
)

var soft404Filler = strings.Repeat("<p>plenty of perfectly ordinary article prose. </p>", 20)

func TestDetectSoft404(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		length    int64
		want      bool
		wantHints []string
	}{
		{
			name:   "healthy long page",
			html:   "<html><head><title>Widgets Weekly</title></head><body>" + soft404Filler + "</body></html>",
			length: 2000,
			want:   false,
		},
		{
			name:      "error text plus error title",
			html:      "<html><head><title>404 Not Found</title></head><body><p>Page not found.</p>" + soft404Filler + "</body></html>",
			length:    2000,
			want:      true,
			wantHints: []string{"error-text:page-not-found", "error-title"},
		},
		{
			name:      "short content alone is not enough",
			html:      "<html><head><title>Hi</title></head><body>ok</body></html>",
			length:    80,
			want:      false,
			wantHints: []string{"very-short-content:80-bytes"},
		},
		{
			name:      "short content plus error text",
			html:      `<html><body>Sorry, we couldn't find that.</body></html>`,
			length:    60,
			want:      true,
			wantHints: []string{"very-short-content:60-bytes", "error-text:sorry,-we-couldn't-find"},
		},
		{
			name:      "error text alone on a long page is not enough",
			html:      "<html><head><title>Archive</title></head><body><p>This article no longer exists in print form.</p>" + soft404Filler + "</body></html>",
			length:    2000,
			want:      false,
			wantHints: []string{"error-text:no-longer-exists"},
		},
		{
			name:      "title-only 404 on a long page is not enough",
			html:      "<html><head><title>404</title></head><body>" + soft404Filler + "</body></html>",
			length:    2000,
			want:      false,
			wantHints: []string{"error-title"},
		},
		{
			name:      "unknown content length with two text signals",
			html:      `<html><head><title>Error</title></head><body>The page doesn't exist.</body></html>`,
			length:    0,
			want:      true,
			wantHints: []string{"error-text:page-doesn't-exist", "error-title"},
		},
		{
			name:      "first matching text pattern wins, only one text hint",
			html:      `<html><body>page not found and also 404 error and content unavailable</body></html>`,
			length:    2000,
			want:      false,
			wantHints: []string{"error-text:page-not-found"},
		},
		{
			name:      "substring false positive: terror in title plus short content",
			html:      `<html><head><title>Terror at the Museum</title></head><body>ok</body></html>`,
			length:    80,
			want:      true,
			wantHints: []string{"very-short-content:80-bytes", "error-title"},
		},
		{
			name:   "empty html with healthy length",
			html:   "",
			length: 2000,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hints := detectSoft404(tt.html, tt.length)
			if got != tt.want {
				t.Errorf("detectSoft404 = %v, want %v (hints=%v)", got, tt.want, hints)
			}
			if !reflect.DeepEqual(hints, tt.wantHints) {
				t.Errorf("hints: got %v want %v", hints, tt.wantHints)
			}
		})
	}
}
