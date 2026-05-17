package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestSnapshotGolden is a table-driven golden-file regression test for the
// accessibility-tree compact format. On mismatch, regenerate with:
//
//	UPDATE_GOLDEN=1 go test ./internal/engine/ -run TestSnapshotGolden
func TestSnapshotGolden(t *testing.T) {
	fixtures := []struct{ path, slug string }{
		{"../../testdata/static/article-ldjson.html", "article-ldjson"},
		{"../../testdata/ssr/mdn-http-methods.html", "mdn-http-methods"},
		{"../../testdata/static/github-awesome.html", "github-awesome"},
		{"../../testdata/ssr/hn-frontpage-fragment.html", "hn-frontpage-fragment"},
		{"../../testdata/static/article-og-full.html", "article-og-full"},
		{"../../testdata/static/gitlab-project.html", "gitlab-project"},
		{"../../testdata/static/charset-latin1.html", "charset-latin1"},
		{"../../testdata/static/charset-shiftjis.html", "charset-shiftjis"},
		{"../../testdata/blocked/cloudflare-challenge.html", "blocked-cloudflare"},
		{"../../testdata/static/wikipedia-latin-phrases.html", "wikipedia-latin-phrases"},
	}
	update := os.Getenv("UPDATE_GOLDEN") == "1"
	for _, f := range fixtures {
		t.Run(f.slug, func(t *testing.T) {
			htmlBytes, err := os.ReadFile(f.path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			tree, err := BuildSnapshot(string(htmlBytes))
			if err != nil {
				t.Fatalf("BuildSnapshot: %v", err)
			}
			got := tree.ToCompact()
			goldenPath := filepath.Join("..", "..", "testdata", "snapshot", "golden", f.slug+".compact")
			if update {
				if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s (%d bytes)", goldenPath, len(got))
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("missing golden %q — regenerate with: UPDATE_GOLDEN=1 go test ./internal/engine/ -run TestSnapshotGolden", goldenPath)
			}
			if string(want) != got {
				t.Errorf("snapshot mismatch for %s.\nRegenerate with: UPDATE_GOLDEN=1 go test ./internal/engine/ -run TestSnapshotGolden\nFirst diff:\n%s",
					f.slug, firstDiffLines(string(want), got, 20))
			}
		})
	}
}

// firstDiffLines returns a short snippet showing where want/got first diverge.
func firstDiffLines(want, got string, n int) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if wantLines[i] != gotLines[i] {
			hi := i + n
			if hi > len(gotLines) {
				hi = len(gotLines)
			}
			return fmt.Sprintf("line %d:\n want: %q\n  got: %q\n... (next %d got lines)\n%s",
				i+1, wantLines[i], gotLines[i], hi-i, strings.Join(gotLines[i:hi], "\n"))
		}
	}
	return "(length difference: want " + strconv.Itoa(len(want)) + " got " + strconv.Itoa(len(got)) + ")"
}
