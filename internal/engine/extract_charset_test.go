package engine

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/pinchtab/seaportal/internal/engine/leakcheck"
)

func serveFixture(t *testing.T, path, contentType string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
	}))
}

func TestExtract_CharsetMatrix(t *testing.T) {
	leakcheck.CheckLeak(t)
	cases := []struct {
		name           string
		fixture        string
		contentType    string
		wantCharset    string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:           "header-correct-latin1",
			fixture:        "../../testdata/static/charset-latin1.html",
			contentType:    "text/html; charset=ISO-8859-1",
			wantCharset:    "iso-8859-1",
			wantContains:   []string{"café", "français", "à", "François"},
			wantNotContain: []string{"Ã©", "Ã "},
		},
		{
			name:         "no-charset-shiftjis",
			fixture:      "../../testdata/static/charset-shiftjis.html",
			contentType:  "text/html",
			wantCharset:  "shift_jis",
			wantContains: []string{"日本語"},
		},
		{
			name:           "header-misdeclared-gb2312",
			fixture:        "../../testdata/static/charset-latin1.html",
			contentType:    "text/html; charset=gb2312",
			wantCharset:    "iso-8859-1",
			wantContains:   []string{"café", "français", "François"},
			wantNotContain: []string{"fran鏰is", "pr鑣", "Fran鏾is"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := serveFixture(t, tc.fixture, tc.contentType)
			defer srv.Close()

			result := FromURL(srv.URL)
			if result.Error != "" {
				t.Fatalf("extract error: %s", result.Error)
			}
			if result.Charset != tc.wantCharset {
				t.Fatalf("Charset = %q, want %q", result.Charset, tc.wantCharset)
			}
			if !utf8.ValidString(result.Content) {
				t.Fatalf("Content is not valid UTF-8")
			}
			for _, want := range tc.wantContains {
				if !strings.Contains(result.Content, want) {
					t.Errorf("Content missing %q\n---\n%s", want, result.Content)
				}
			}
			for _, bad := range tc.wantNotContain {
				if strings.Contains(result.Content, bad) {
					t.Errorf("Content unexpectedly contains %q\n---\n%s", bad, result.Content)
				}
			}
		})
	}
}
