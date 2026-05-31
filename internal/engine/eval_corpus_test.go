package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// corpusPath is the eval corpus relative to this package's directory.
const corpusPath = "../../tests/eval/corpus.yaml"

// repoRoot points back to the seaportal repo root from internal/engine; used
// to resolve corpus entry paths which are stored repo-relative.
const repoRoot = "../.."

func TestLoadCorpus_ParsesCleanly(t *testing.T) {
	entries, err := LoadCorpus(corpusPath)
	if err != nil {
		t.Fatalf("LoadCorpus(%q): %v", corpusPath, err)
	}
	if len(entries) < 25 {
		t.Fatalf("expected ≥ 25 corpus entries, got %d", len(entries))
	}
	for i, e := range entries {
		if e.Path == "" {
			t.Errorf("entry %d: empty Path", i)
		}
		if len(e.MustInclude) == 0 && len(e.MustExclude) == 0 {
			t.Errorf("entry %d (%s): both must_include and must_exclude empty", i, e.Path)
		}
		if e.ExpectClass == "" {
			t.Errorf("entry %d (%s): empty expect_class", i, e.Path)
		}
	}
}

func TestLoadCorpus_AllFixturesExist(t *testing.T) {
	entries, err := LoadCorpus(corpusPath)
	if err != nil {
		t.Fatalf("LoadCorpus(%q): %v", corpusPath, err)
	}
	for _, e := range entries {
		abs := filepath.Join(repoRoot, e.Path)
		info, err := os.Stat(abs)
		if err != nil {
			t.Errorf("fixture missing for entry %q: %v", e.Path, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("fixture path is a directory: %q", e.Path)
		}
	}
}
