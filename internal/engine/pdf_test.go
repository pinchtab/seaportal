package engine

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadSamplePDF reads testdata/sample.pdf walking up from CWD to find the repo
// testdata directory (tests run from the package dir).
func loadSamplePDF(t *testing.T) []byte {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "..", "testdata", "sample.pdf"),
		filepath.Join("testdata", "sample.pdf"),
	}
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil {
			return b
		}
	}
	t.Fatalf("could not locate testdata/sample.pdf in %v", candidates)
	return nil
}

func TestExtractPDFText_MinimalSinglePage(t *testing.T) {
	body := loadSamplePDF(t)
	out, err := ExtractPDFText(body)
	if err != nil {
		t.Fatalf("ExtractPDFText: %v", err)
	}
	if !strings.Contains(out, "Hello world") {
		t.Fatalf("expected 'Hello world' in extracted text, got: %q", out)
	}
	if !strings.Contains(out, "--- page 1 ---") {
		t.Fatalf("expected page marker in output, got: %q", out)
	}
}

func TestExtractPDFText_EmptyBody(t *testing.T) {
	_, err := ExtractPDFText([]byte{})
	if err == nil {
		t.Fatalf("expected error for empty body")
	}
}

func TestExtractPDFText_MalformedPDF(t *testing.T) {
	_, err := ExtractPDFText([]byte("not a pdf at all"))
	if err == nil {
		t.Fatalf("expected error for malformed input")
	}
}

func TestExtractPDFText_HandlesNullPage(t *testing.T) {
	// Build a PDF that claims one page but the content stream is malformed.
	// safeGetPlainText should recover and emit an "[extraction failed: ...]"
	// marker rather than aborting.
	body := loadSamplePDF(t)
	// Corrupt the content stream by replacing a printable byte inside the
	// BT...ET block with junk. The reader still parses the structure but
	// GetPlainText may panic / error — either way we want a clean outcome.
	corrupted := make([]byte, len(body))
	copy(corrupted, body)
	idx := strings.Index(string(corrupted), "BT /F1")
	if idx > 0 {
		// Mangle the operator. Pdf still parses; text extraction may fail.
		corrupted[idx] = 0x01
		corrupted[idx+1] = 0x02
	}
	out, err := ExtractPDFText(corrupted)
	// Either ExtractPDFText errors out (acceptable for severe corruption) or
	// it returns a string with the extraction-failed marker (preferred).
	if err == nil && !strings.Contains(out, "page 1") {
		t.Fatalf("expected page marker even when extraction degrades, got: %q", out)
	}
}

func TestExtract_PDFContentType(t *testing.T) {
	body := loadSamplePDF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	res := FromURLWithOptions(srv.URL, Options{})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if res.ExtractionMethod != "pdf" {
		t.Fatalf("expected ExtractionMethod=pdf, got %q", res.ExtractionMethod)
	}
	if !strings.Contains(res.Content, "Hello world") {
		t.Fatalf("expected 'Hello world' in result.Content, got: %q", res.Content)
	}
	if res.Title == "" {
		t.Fatalf("expected non-empty Title")
	}
	if res.ResponseContentType == "" || !strings.Contains(res.ResponseContentType, "application/pdf") {
		t.Fatalf("expected pdf content-type recorded, got %q", res.ResponseContentType)
	}
}

func TestExtract_NoPDFFlagSkips(t *testing.T) {
	body := loadSamplePDF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	res := FromURLWithOptions(srv.URL, Options{NoPDF: true})
	if res.Error == "" {
		t.Fatalf("expected error when --no-pdf set on a PDF response")
	}
	if !strings.Contains(res.Error, "binary") && !strings.Contains(res.Error, "pdf") {
		t.Fatalf("expected binary-content error, got: %q", res.Error)
	}
	if res.ExtractionMethod == "pdf" {
		t.Fatalf("did not expect pdf extraction when NoPDF=true")
	}
	if strings.Contains(res.Content, "Hello world") {
		t.Fatalf("did not expect content extraction when NoPDF=true")
	}
}

// regression: pdf-malformed-xref-panic
//
// Surfaced by FuzzPDF. The upstream `ledongthuc/pdf` library panics
// (rather than returning an error) when `startxref` points past EOF.
// Before the fix, ExtractPDFText's only recover was inside
// safeGetPlainText (per-page), so the panic from NewReader/NumPage
// killed the whole process. Now the top-level defer recover() converts
// the panic into an error.
func TestExtractPDFText_MalformedXrefReturnsError(t *testing.T) {
	// %PDF-1.0\n + 96 zero bytes + \nstartxref\n100%%EOF — startxref
	// claims offset 100 but the body is shorter; upstream panics.
	var body []byte
	body = append(body, []byte("%PDF-1.0\n")...)
	body = append(body, make([]byte, 96)...)
	body = append(body, []byte("\nstartxref\n100%%EOF")...)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ExtractPDFText panicked on malformed xref: %v", r)
		}
	}()
	out, err := ExtractPDFText(body)
	if err == nil {
		t.Fatalf("expected error on malformed xref, got nil (out len=%d)", len(out))
	}
	if out != "" {
		t.Errorf("expected empty output on error, got %d bytes", len(out))
	}
}
