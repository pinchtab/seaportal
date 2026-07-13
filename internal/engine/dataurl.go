package engine

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

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

	mime = "text/plain"
	isBase64 := false
	if meta != "" {
		parts := strings.Split(meta, ";")
		if parts[0] != "" && !strings.Contains(parts[0], "=") && parts[0] != "base64" {
			mime = strings.ToLower(parts[0])
		}
		for _, p := range parts[1:] {
			if p == "base64" {
				isBase64 = true
			}
		}
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
	decoded, decErr := url.QueryUnescape(data)
	if decErr != nil {
		return "", nil, fmt.Errorf("data URL percent-decode: %w", decErr)
	}
	return mime, []byte(decoded), nil
}
