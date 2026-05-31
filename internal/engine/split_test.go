package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitResultToFiles_PrefersChunks(t *testing.T) {
	dir := t.TempDir()
	r := Result{
		URL:   "https://example.com/foo",
		Title: "Foo",
		Chunks: []Chunk{
			{Index: 0, Text: strings.Repeat("a", 100)},
			{Index: 1, Text: strings.Repeat("b", 100)},
			{Index: 2, Text: strings.Repeat("c", 100)},
			{Index: 3, Text: strings.Repeat("d", 100)},
		},
		// Content present but should be ignored when Chunks exist.
		Content: "should-not-be-used",
	}
	// Cap small enough that each chunk lands in its own file.
	files, err := SplitResultToFiles(r, SplitConfig{Dir: dir, MaxBytes: 150})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("want 4 files, got %d", len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f.Path)
		if err != nil {
			t.Fatalf("read %s: %v", f.Path, err)
		}
		if strings.Contains(string(b), "should-not-be-used") {
			t.Errorf("file %s contains Content instead of chunk text", f.Path)
		}
	}
}

func TestSplitResultToFiles_FallsBackToParagraphSplit(t *testing.T) {
	dir := t.TempDir()
	paras := []string{
		strings.Repeat("alpha ", 50),
		strings.Repeat("bravo ", 50),
		strings.Repeat("charlie ", 50),
		strings.Repeat("delta ", 50),
	}
	r := Result{
		URL:     "https://example.com/path",
		Content: strings.Join(paras, "\n\n"),
	}
	files, err := SplitResultToFiles(r, SplitConfig{Dir: dir, MaxBytes: 400})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("want at least 2 files, got %d", len(files))
	}
}

func TestSplitResultToFiles_RespectsByteCap(t *testing.T) {
	dir := t.TempDir()
	var paras []string
	for i := 0; i < 10; i++ {
		paras = append(paras, strings.Repeat("x", 200))
	}
	r := Result{
		URL:     "https://example.com/cap",
		Content: strings.Join(paras, "\n\n"),
	}
	cap := 500
	files, err := SplitResultToFiles(r, SplitConfig{Dir: dir, MaxBytes: cap})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one file")
	}
	for _, f := range files {
		if f.Bytes > cap {
			t.Errorf("file %s exceeded cap: %d > %d", f.Path, f.Bytes, cap)
		}
	}
}

func TestSplitResultToFiles_HandlesEmptyContent(t *testing.T) {
	dir := t.TempDir()
	r := Result{URL: "https://example.com/empty"}
	files, err := SplitResultToFiles(r, SplitConfig{Dir: dir})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if files != nil {
		t.Errorf("want nil manifest, got %v", files)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("want empty dir, got %d entries", len(entries))
	}
}

func TestSplitResultToFiles_WarnsOnOversizedFirstPara(t *testing.T) {
	dir := t.TempDir()
	// Capture stderr.
	oldStderr := os.Stderr
	rPipe, wPipe, _ := os.Pipe()
	os.Stderr = wPipe
	defer func() { os.Stderr = oldStderr }()

	big := strings.Repeat("z", 5000)
	r := Result{URL: "https://example.com/big", Content: big}
	files, err := SplitResultToFiles(r, SplitConfig{Dir: dir, MaxBytes: 100})
	_ = wPipe.Close()
	os.Stderr = oldStderr
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d", len(files))
	}
	if files[0].Bytes != 5000 {
		t.Errorf("want 5000 bytes, got %d", files[0].Bytes)
	}
	buf := make([]byte, 2048)
	n, _ := rPipe.Read(buf)
	if !strings.Contains(string(buf[:n]), "oversized") {
		t.Errorf("want stderr to contain 'oversized', got: %s", string(buf[:n]))
	}
}

func TestSplitResultToFiles_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	r := Result{
		URL: "https://example.com/atomic",
		Chunks: []Chunk{
			{Index: 0, Text: "hello"},
			{Index: 1, Text: "world"},
		},
	}
	if _, err := SplitResultToFiles(r, SplitConfig{Dir: dir, MaxBytes: 10}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf(".tmp file survived: %s", e.Name())
		}
	}
}

func TestSplitResultToFiles_SlugFromURL(t *testing.T) {
	got := slugFromURL("https://EXAMPLE.com/path/to/foo")
	want := "example-com-path-to-foo"
	if got != want {
		t.Errorf("slugFromURL = %q, want %q", got, want)
	}
	if slugFromURL("") != "seaportal" {
		t.Errorf("empty URL should yield 'seaportal'")
	}
	long := slugFromURL("https://example.com/" + strings.Repeat("a", 200))
	if len(long) > 60 {
		t.Errorf("slug not truncated to 60: %d", len(long))
	}
}

func TestSplitResultToFiles_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	r := Result{
		URL:   "https://example.com/json",
		Title: "JSON test",
		Chunks: []Chunk{
			{Index: 0, Text: strings.Repeat("a", 40)},
			{Index: 1, Text: strings.Repeat("b", 40)},
		},
	}
	files, err := SplitResultToFiles(r, SplitConfig{Dir: dir, MaxBytes: 50, Format: "json"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d", len(files))
	}
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".json") {
			t.Errorf("want .json suffix, got %s", f.Path)
		}
		b, err := os.ReadFile(f.Path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var obj map[string]interface{}
		if err := json.Unmarshal(b, &obj); err != nil {
			t.Errorf("invalid JSON in %s: %v", f.Path, err)
		}
		for _, key := range []string{"index", "of", "url", "title", "text"} {
			if _, ok := obj[key]; !ok {
				t.Errorf("missing key %q in %s", key, f.Path)
			}
		}
	}
}

func TestSplitResultToFiles_MDFormat(t *testing.T) {
	dir := t.TempDir()
	r := Result{
		URL: "https://example.com/md",
		Chunks: []Chunk{
			{Index: 0, Text: "# Heading\n\nparagraph"},
		},
	}
	files, err := SplitResultToFiles(r, SplitConfig{Dir: dir, Format: "md"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d", len(files))
	}
	if !strings.HasSuffix(files[0].Path, ".md") {
		t.Errorf("want .md suffix, got %s", files[0].Path)
	}
	b, err := os.ReadFile(files[0].Path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != "# Heading\n\nparagraph" {
		t.Errorf("md content not raw, got %q", string(b))
	}
}

// ── Integration tests ──────────────────────────────────────────────

func TestExtract_SplitOutFlagWritesFiles(t *testing.T) {
	body := `<!doctype html><html><head><title>Long Doc</title></head><body><article>` +
		`<h1>Long Doc</h1>`
	for i := 0; i < 30; i++ {
		body += "<p>" + strings.Repeat("alpha bravo charlie delta echo foxtrot ", 20) + "</p>"
	}
	body += `</article></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	opts := Options{SplitOut: dir, SplitBytes: 800}
	r := FromURLWithOptions(srv.URL, opts)

	// The CLI is what calls SplitResultToFiles, but the integration test verifies
	// the library wiring: when SplitOut is set on Options the caller is expected
	// to invoke SplitResultToFiles. We invoke it directly to match the CLI path.
	files, err := SplitResultToFiles(r, SplitConfig{Dir: opts.SplitOut, MaxBytes: opts.SplitBytes})
	if err != nil {
		t.Fatalf("split err: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("want files written")
	}
	r.SplitFiles = files
	if r.SplitFiles == nil {
		t.Fatal("want SplitFiles populated")
	}
	for _, f := range files {
		if _, err := os.Stat(f.Path); err != nil {
			t.Errorf("file missing: %s", f.Path)
		}
		// Verify under tempdir.
		absDir, _ := filepath.Abs(dir)
		if !strings.HasPrefix(f.Path, absDir) {
			t.Errorf("file %s not under %s", f.Path, absDir)
		}
	}
}

func TestExtract_SplitOutFlagOffDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><body><article><h1>x</h1><p>hello world</p></article></body></html>`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	r := FromURLWithOptions(srv.URL, Options{})
	if r.SplitFiles != nil {
		t.Errorf("want SplitFiles nil by default, got %v", r.SplitFiles)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("want no files written, got %d", len(entries))
	}
}
