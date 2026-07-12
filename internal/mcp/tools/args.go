package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pinchtab/seaportal"
)

// requiredURLArg returns args[key] as a non-empty string, or the canonical
// missing-argument error every URL-taking tool reports.
func requiredURLArg(args map[string]interface{}, key string) (string, error) {
	s, _ := args[key].(string)
	if s == "" {
		return "", fmt.Errorf("missing required argument: %s", key)
	}
	return s, nil
}

// securityFromArgs returns the secure-by-default policy, with the private-IP
// block lifted when the allow_internal argument is true.
func securityFromArgs(args map[string]interface{}) *seaportal.SecurityPolicy {
	sec := seaportal.DefaultSecurityPolicy()
	if v, ok := args["allow_internal"].(bool); ok && v {
		sec.BlockPrivateIPs = false
	}
	return sec
}

// marshalResult JSON-encodes a tool result, naming the payload on failure
// (e.g. "marshal snapshot: ...").
func marshalResult(v interface{}, what string) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", what, err)
	}
	return string(b), nil
}

// argInt reads a JSON-RPC numeric argument (float64) as an int, or def when
// absent/non-positive.
func argInt(args map[string]interface{}, key string, def int) int {
	if v, ok := args[key].(float64); ok && int(v) > 0 {
		return int(v)
	}
	return def
}

func argString(args map[string]interface{}, key string) string {
	s, _ := args[key].(string)
	return s
}

// splitCSV splits a comma-separated argument into trimmed, non-empty items.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// fetchHTML retrieves url through the shared security-guarded fetch path with
// a browser-style Accept header, bound to the handler context so client
// cancellation stops the fetch.
func fetchHTML(ctx context.Context, url string, sec *seaportal.SecurityPolicy) (string, error) {
	body, _, _, err := seaportal.FetchBytes(ctx, url, seaportal.FetchBytesOptions{
		Timeout:  30 * time.Second,
		Security: sec,
		Accept:   "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	})
	if err != nil {
		return "", err
	}
	return string(body), nil
}
