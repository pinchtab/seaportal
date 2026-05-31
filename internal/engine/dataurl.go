package engine

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// parseDataURL parses RFC 2397 "data:<mediatype>[;base64],<data>" URLs.
// Returns the lowercased media type (without parameters), the decoded
// body bytes, and an error on malformed input. Stdlib-only (encoding/base64
// + net/url) — no external deps. Hostname-agnostic.
func parseDataURL(s string) (mime string, body []byte, err error) {
	if !strings.HasPrefix(s, "data:") {
		return "", nil, fmt.Errorf("not a data URL")
	}
	rest := s[len("data:"):]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", nil, fmt.Errorf("data URL missing comma")
	}
	meta := rest[:comma]
	data := rest[comma+1:]

	// Default mime is text/plain;charset=US-ASCII per RFC 2397.
	mime = "text/plain"
	isBase64 := false
	if meta != "" {
		// meta may be `<mediatype>[;param=value]*[;base64]`.
		parts := strings.Split(meta, ";")
		// First segment is the mediatype only if it doesn't look like a
		// `key=value` param and isn't the bare `base64` token (the
		// `data:;base64,...` shape leaves an empty leading segment).
		if parts[0] != "" && !strings.Contains(parts[0], "=") && parts[0] != "base64" {
			mime = strings.ToLower(parts[0])
		}
		for _, p := range parts[1:] {
			if p == "base64" {
				isBase64 = true
			}
		}
		// Handle the `data:;base64,...` edge: first segment is empty,
		// but the bare `base64` token may live in parts[0].
		if parts[0] == "base64" {
			isBase64 = true
		}
	}

	if isBase64 {
		decoded, decErr := base64.StdEncoding.DecodeString(data)
		if decErr != nil {
			return "", nil, fmt.Errorf("data URL base64 decode: %w", decErr)
		}
		return mime, decoded, nil
	}
	// RFC 2397 says the data part is URL-encoded; decode percent-escapes.
	decoded, decErr := url.QueryUnescape(data)
	if decErr != nil {
		return "", nil, fmt.Errorf("data URL percent-decode: %w", decErr)
	}
	return mime, []byte(decoded), nil
}
