package engine

import (
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/htmlindex"
)

// mojibakeThreshold is the fraction of suspect runes (see mojibakeRatio)
// above which a decoded body is considered mojibake-y enough to warrant a
// retry with the body's own <meta> charset.
//
// Why 5%? — measured against the canonical failure mode (latin-1 French
// body decoded as gb2312, scored over non-ASCII runes only), the suspect
// density lands well above 5%. Correctly-decoded prose in Latin scripts
// has near-zero CJK / U+FFFD / control-char runes, so it stays well below.
// The threshold is intentionally conservative: a false-positive costs one
// extra decode pass; a false-negative ships silent CJK mojibake to the
// user.
const mojibakeThreshold = 0.05

// Sniff window for <meta charset> / <meta http-equiv> declarations.
const charsetSniffWindow = 1024

var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16BE = []byte{0xFE, 0xFF}
	bomUTF16LE = []byte{0xFF, 0xFE}

	// <meta charset="…"> — case-insensitive, attribute may be quoted or bare.
	metaCharsetRE = regexp.MustCompile(`(?i)<meta[^>]+charset\s*=\s*["']?\s*([A-Za-z0-9_:.\-]+)`)
	// <meta http-equiv="Content-Type" content="…; charset=…"> — pull the charset segment.
	metaHTTPEquivRE = regexp.MustCompile(`(?i)<meta[^>]+http-equiv\s*=\s*["']?content-type["']?[^>]*content\s*=\s*["'][^"']*charset\s*=\s*([A-Za-z0-9_:.\-]+)`)
	// charset=… inside a Content-Type header value.
	contentTypeCharsetRE = regexp.MustCompile(`(?i)charset\s*=\s*["']?\s*([A-Za-z0-9_:.\-]+)`)
)

// normalizeCharset lowercases, trims whitespace and surrounding quotes.
func normalizeCharset(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	s = strings.TrimSpace(s)
	return strings.ToLower(s)
}

// detectCharset walks the priority chain: BOM → Content-Type header → <meta charset> →
// <meta http-equiv>. Returns "" when no signal was found (caller should treat as UTF-8).
func detectCharset(body []byte, contentType string) string {
	// 1. BOM
	if bytes.HasPrefix(body, bomUTF8) {
		return "utf-8"
	}
	if bytes.HasPrefix(body, bomUTF16BE) {
		return "utf-16be"
	}
	if bytes.HasPrefix(body, bomUTF16LE) {
		return "utf-16le"
	}

	// 2. Content-Type header
	if contentType != "" {
		if m := contentTypeCharsetRE.FindStringSubmatch(contentType); len(m) > 1 {
			if cs := normalizeCharset(m[1]); cs != "" {
				return cs
			}
		}
	}

	// 3 + 4. <meta charset> / <meta http-equiv> in first 1024 bytes.
	head := body
	if len(head) > charsetSniffWindow {
		head = head[:charsetSniffWindow]
	}
	if m := metaCharsetRE.FindSubmatch(head); len(m) > 1 {
		if cs := normalizeCharset(string(m[1])); cs != "" {
			return cs
		}
	}
	if m := metaHTTPEquivRE.FindSubmatch(head); len(m) > 1 {
		if cs := normalizeCharset(string(m[1])); cs != "" {
			return cs
		}
	}

	return ""
}

// decodeBytes converts body from the named charset to UTF-8. UTF-8 input is
// returned unchanged (only BOM is stripped). On unknown/failed decode, returns
// the input untouched with the error so the caller can decide whether to warn.
func decodeBytes(body []byte, charset string) ([]byte, error) {
	switch charset {
	case "":
		return body, nil
	case "utf-8", "utf8":
		// Strip BOM if present, otherwise pass through.
		return bytes.TrimPrefix(body, bomUTF8), nil
	case "utf-16be":
		body = bytes.TrimPrefix(body, bomUTF16BE)
	case "utf-16le":
		body = bytes.TrimPrefix(body, bomUTF16LE)
	}

	enc, err := htmlindex.Get(charset)
	if err != nil {
		return body, err
	}
	decoded, err := enc.NewDecoder().Bytes(body)
	if err != nil {
		return body, err
	}
	return decoded, nil
}

// metaOnly returns the body's <meta charset> / <meta http-equiv> declaration
// (or "" if absent), bypassing the Content-Type header entirely. Used by the
// post-decode recovery path to detect header/meta disagreement.
func metaOnly(body []byte) string {
	head := body
	if len(head) > charsetSniffWindow {
		head = head[:charsetSniffWindow]
	}
	if m := metaCharsetRE.FindSubmatch(head); len(m) > 1 {
		if cs := normalizeCharset(string(m[1])); cs != "" {
			return cs
		}
	}
	if m := metaHTTPEquivRE.FindSubmatch(head); len(m) > 1 {
		if cs := normalizeCharset(string(m[1])); cs != "" {
			return cs
		}
	}
	return ""
}

// mojibakeRatio returns the fraction of "suspect" runes in s, scored over
// the non-ASCII rune population only (HTML markup is overwhelmingly ASCII
// regardless of body encoding, so including it would dilute the signal
// below any reasonable threshold).
//
// A rune is "suspect" when it is:
//   - U+FFFD (Unicode replacement char — explicit decode error), or
//   - a stray ASCII control char (< 0x20, excluding \t \n \r — counted in
//     the numerator AND denominator so single-byte garbage is visible), or
//   - a CJK Unified Ideograph (U+4E00..U+9FFF). These dominate when a
//     Latin-encoded body is forced through a CJK codec; the recovery gate
//     (meta charset disagrees with header) keeps genuine CJK pages — where
//     header and meta normally agree — out of the retry path.
//
// Returns 0 for empty input or input with no non-ASCII / non-suspect runes
// (a pure-ASCII document is unambiguous, no need to retry).
func mojibakeRatio(s []byte) float64 {
	if len(s) == 0 {
		return 0
	}
	bad := 0
	denom := 0
	for _, r := range string(s) {
		isControl := r < 0x20 && r != '\t' && r != '\n' && r != '\r'
		isReplacement := r == utf8.RuneError
		isCJK := r >= 0x4E00 && r <= 0x9FFF
		nonASCII := r > 0x7F
		if isControl || isReplacement {
			bad++
			denom++
			continue
		}
		if nonASCII {
			denom++
			if isCJK {
				bad++
			}
		}
	}
	if denom == 0 {
		return 0
	}
	return float64(bad) / float64(denom)
}

// sniffAndDecode detects the body charset and decodes it to UTF-8. Returns
// ok=true only when a non-trivial decode actually happened (i.e. the bytes
// changed or a non-empty charset label was detected); callers can use this to
// decide whether to surface Result.Charset.
//
// Recovery: when the header-declared charset disagrees with the body's own
// <meta> tag AND the header-driven decode produces mojibake density above
// mojibakeThreshold, retry the decode using the meta-declared charset and
// keep whichever output is cleaner. This rescues pages served by misconfigured
// CMSes (Apache AddDefaultCharset, CDN-injected gb2312, etc.) without
// hostname-specific heuristics.
func sniffAndDecode(body []byte, contentType string) ([]byte, string, bool) {
	cs := detectCharset(body, contentType)
	if cs == "" {
		return body, "", false
	}
	decoded, err := decodeBytes(body, cs)
	if err != nil {
		// Failure mode: pass through unchanged, don't surface a charset.
		return body, "", false
	}

	// Post-decode validation: if the body's <meta> tag disagrees with the
	// header charset and the header-driven decode looks like mojibake, try
	// the meta charset and keep the cleaner result.
	if metaCS := metaOnly(body); metaCS != "" && metaCS != cs {
		headerRatio := mojibakeRatio(decoded)
		if headerRatio > mojibakeThreshold {
			if recovered, rerr := decodeBytes(body, metaCS); rerr == nil {
				if mojibakeRatio(recovered) < headerRatio {
					return recovered, metaCS, true
				}
			}
		}
	}

	return decoded, cs, true
}

// isCharsetSniffableContentType returns true when the response body is
// HTML/plain-text-shaped and worth running through the charset sniff.
func isCharsetSniffableContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	// Strip parameters.
	if idx := strings.Index(ct, ";"); idx >= 0 {
		ct = ct[:idx]
	}
	ct = strings.TrimSpace(ct)
	switch ct {
	case "", "text/html", "text/plain", "application/xhtml+xml":
		return true
	}
	return false
}
