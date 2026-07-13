package engine

import (
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/htmlindex"
)

const mojibakeThreshold = 0.05

const charsetSniffWindow = 1024

var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16BE = []byte{0xFE, 0xFF}
	bomUTF16LE = []byte{0xFF, 0xFE}

	metaCharsetRE        = regexp.MustCompile(`(?i)<meta[^>]+charset\s*=\s*["']?\s*([A-Za-z0-9_:.\-]+)`)
	metaHTTPEquivRE      = regexp.MustCompile(`(?i)<meta[^>]+http-equiv\s*=\s*["']?content-type["']?[^>]*content\s*=\s*["'][^"']*charset\s*=\s*([A-Za-z0-9_:.\-]+)`)
	contentTypeCharsetRE = regexp.MustCompile(`(?i)charset\s*=\s*["']?\s*([A-Za-z0-9_:.\-]+)`)
)

func normalizeCharset(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	s = strings.TrimSpace(s)
	return strings.ToLower(s)
}

func detectCharset(body []byte, contentType string) string {
	if bytes.HasPrefix(body, bomUTF8) {
		return "utf-8"
	}
	if bytes.HasPrefix(body, bomUTF16BE) {
		return "utf-16be"
	}
	if bytes.HasPrefix(body, bomUTF16LE) {
		return "utf-16le"
	}

	if contentType != "" {
		if m := contentTypeCharsetRE.FindStringSubmatch(contentType); len(m) > 1 {
			if cs := normalizeCharset(m[1]); cs != "" {
				return cs
			}
		}
	}

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

func decodeBytes(body []byte, charset string) ([]byte, error) {
	switch charset {
	case "":
		return body, nil
	case "utf-8", "utf8":
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

func sniffAndDecode(body []byte, contentType string) ([]byte, string, bool) {
	cs := detectCharset(body, contentType)
	if cs == "" {
		return body, "", false
	}
	decoded, err := decodeBytes(body, cs)
	if err != nil {
		return body, "", false
	}

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

func isCharsetSniffableContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
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
