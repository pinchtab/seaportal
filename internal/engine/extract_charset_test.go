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

// TestExtract_CharsetMatrix exercises the three realistic header/meta
// combinations for non-UTF-8 source bodies:
//
//  1. header-correct-latin1     — Content-Type declares iso-8859-1 (matches body).
//  2. no-charset-shiftjis       — Content-Type omits charset; <meta charset>
//     in the body is the only signal (shift_jis).
//  3. header-misdeclared-gb2312 — Content-Type lies (gb2312) about an
//     iso-8859-1 body whose <meta http-equiv> correctly says ISO-8859-1.
//
// Case (3) exercises the post-decode recovery path in sniffAndDecode: the
// header-declared gb2312 decode produces CJK mojibake on the French body
// (high U+FFFD / control-char density), the body's <meta http-equiv> says
// ISO-8859-1, the two disagree, and the meta-driven re-decode is cleaner —
// so the recovered charset wins. The previously locked-in mojibake markers
// (鏰 / 鑣 / 鏾) must no longer appear.
func TestExtract_CharsetMatrix(t *testing.T) {
	leakcheck.CheckLeak(t)
	cases := []struct {
		name           string
		fixture        string
		contentType    string
		wantCharset    string
		wantContains   []string
		wantNotContain []string
		// mustBeValidUTF8 always true — decoders are required to emit UTF-8
		// even when the chosen encoding is wrong.
	}{
		{
			// Header is authoritative and correct — straight latin-1 → UTF-8.
			name:         "header-correct-latin1",
			fixture:      "../../testdata/static/charset-latin1.html",
			contentType:  "text/html; charset=ISO-8859-1",
			wantCharset:  "iso-8859-1",
			wantContains: []string{"café", "français", "à", "François"},
			// Mojibake guard: if decode failed, "é" would surface as "Ã©".
			wantNotContain: []string{"Ã©", "Ã "},
		},
		{
			// No charset on the wire — sniff falls back to <meta charset>.
			name:         "no-charset-shiftjis",
			fixture:      "../../testdata/static/charset-shiftjis.html",
			contentType:  "text/html",
			wantCharset:  "shift_jis",
			wantContains: []string{"日本語"},
		},
		{
			// LOCK-IN of current behaviour: header beats meta, so a lying
			// gb2312 header decodes latin-1 bytes through the GBK table and
			// the French text comes out as CJK mojibake. The body's own
			// <meta http-equiv="Content-Type" ... charset=ISO-8859-1"> is
			// IGNORED because step 2 (header) of detectCharset succeeded.
			//
			// Documented limitation, NOT desired behaviour. A real recovery
			// pass would either:
			//   - prefer <meta> when the header's charset is unknown/rare, OR
			//   - validate the decoded output and retry on high replacement-
			//     char density.
			// Both are out of scope for this observational test. See
			// todo.md "Mis-declared Content-Type charset recovery" for the
			// follow-up.
			name:        "header-misdeclared-gb2312",
			fixture:     "../../testdata/static/charset-latin1.html",
			contentType: "text/html; charset=gb2312",
			// Recovery: header lies, decoded output is CJK mojibake, the
			// body's <meta http-equiv> truthfully says ISO-8859-1, retry
			// wins.
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
