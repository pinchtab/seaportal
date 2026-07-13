//go:build baseline

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBaselineCapture(t *testing.T) {
	type fixture struct {
		path string
		url  string
	}
	fixtures := []fixture{
		{"linkedin-loggedout.html", "https://www.linkedin.com/"},
		{"gitlab-project.html", "https://gitlab.com/gitlab-org/gitlab"},
		{"wikipedia-latin-phrases.html", "https://en.wikipedia.org/wiki/List_of_Latin_phrases_(full)"},
		{"mdn-http-methods.html", "https://developer.mozilla.org/en-US/docs/Web/HTTP/Methods"},
		{"mdn-http-auth.html", "https://developer.mozilla.org/en-US/docs/Web/HTTP/Authentication"},
		{"github-awesome.html", "https://github.com/sindresorhus/awesome"},
		{"arxiv-attention.html", "https://arxiv.org/abs/1706.03762"},
	}
	for _, f := range fixtures {
		path := filepath.Join("..", "..", "testdata", f.path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Logf("MISSING %s: %v", f.path, err)
			continue
		}
		r := FromHTML(string(data), f.url)
		preview := r.Content
		if len(preview) > 120 {
			preview = preview[:120]
		}
		t.Logf("FIXTURE %s len=%d title=%q preview=%q", f.path, r.Length, r.Title, preview)
	}
}
