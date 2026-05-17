package engine

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// containsWarning is a small helper: returns true when any warning entry
// contains the given substring (case-sensitive).
func containsWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// TestExtract_WarningOnCharsetDecodeFailure declares an unknown charset on a
// synthetic HTML page. The extraction must succeed (raw UTF-8 bytes pass
// through), and a "charset decode" warning must appear.
func TestExtract_WarningOnCharsetDecodeFailure(t *testing.T) {
	const body = `<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=bogus-9999">
<title>Charset Test</title>
</head>
<body>
<article>
<h1>Hello World</h1>
<p>This is a paragraph of plain ASCII text long enough for readability to keep it. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.</p>
<p>Another paragraph: Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.</p>
</article>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Omit charset on the wire so the in-body <meta http-equiv> wins the
		// detection chain (which makes the bogus label trigger the warning).
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	result := FromURLWithOptions(server.URL, Options{})

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Content == "" {
		t.Fatalf("expected non-empty content, got empty")
	}
	if !containsWarning(result.Warnings, "charset decode") {
		t.Errorf("expected a 'charset decode' warning, got: %v", result.Warnings)
	}
}

// TestExtract_WarningOnCacheWriteFailure points CacheDir at a read-only
// directory so atomicWrite fails after Get succeeds. The fetch itself
// completes; the cache write surfaces as a "cache write" warning.
func TestExtract_WarningOnCacheWriteFailure(t *testing.T) {
	const body = `<!DOCTYPE html>
<html>
<head><title>Cache Test</title></head>
<body>
<article>
<h1>Cache Write Warning</h1>
<p>Plain article body long enough to satisfy readability extraction. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p>
<p>Second paragraph: Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit.</p>
</article>
</body>
</html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	// Build a writable temp dir first so NewDiskCache succeeds (it MkdirAll's
	// the directory), then chmod to read-only to break the body/headers writes.
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	t.Cleanup(func() {
		// Restore so t.TempDir's auto-cleanup can recurse.
		_ = os.Chmod(dir, 0o700)
	})

	result := FromURLWithOptions(server.URL, Options{CacheDir: dir})

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !containsWarning(result.Warnings, "cache write") {
		t.Errorf("expected a 'cache write' warning, got: %v", result.Warnings)
	}
}

// TestExtract_NoWarningsOnHappyPath asserts that a clean fetch with no flags
// produces nil/empty Warnings. Regression guard against accidentally wiring
// warnings into the success path.
func TestExtract_NoWarningsOnHappyPath(t *testing.T) {
	const body = `<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
<title>Happy Path</title>
</head>
<body>
<article>
<h1>Happy Path</h1>
<p>Article body long enough for readability to keep it. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p>
<p>Second paragraph: Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate.</p>
</article>
</body>
</html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	result := FromURLWithOptions(server.URL, Options{})

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", result.Warnings)
	}
}
