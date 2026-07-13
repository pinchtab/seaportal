package engine

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var trackingParams = map[string]bool{
	"utm_source":          true,
	"utm_medium":          true,
	"utm_campaign":        true,
	"utm_term":            true,
	"utm_content":         true,
	"utm_id":              true,
	"utm_brand":           true,
	"utm_creative_format": true,

	"fbclid":  true,
	"gclid":   true,
	"dclid":   true,
	"gbraid":  true,
	"wbraid":  true,
	"msclkid": true,
	"yclid":   true,

	"_ga":     true,
	"_gl":     true,
	"_hsenc":  true,
	"_hsmi":   true,
	"mkt_tok": true,
	"mc_eid":  true,
	"mc_cid":  true,

	"igshid":  true,
	"ref":     true,
	"ref_src": true,
	"ref_url": true,
	"s":       true,
	"t":       true,
	"feature": true,

	"pk_campaign": true,
	"pk_kwd":      true,
	"cmpid":       true,
	"wt.mc_id":    true,
}

var trackingPrefixes = []string{"_ga", "_gid", "_fb", "_hs", "utm_"}

var (
	canonicalLinkRelFirstRE  = regexp.MustCompile(`(?is)<link\b[^>]*\brel\s*=\s*["']?canonical["']?[^>]*\bhref\s*=\s*["']([^"'>\s]+)["']?[^>]*>`)
	canonicalLinkHrefFirstRE = regexp.MustCompile(`(?is)<link\b[^>]*\bhref\s*=\s*["']([^"'>\s]+)["'][^>]*\brel\s*=\s*["']?canonical["']?[^>]*>`)

	multiSlashRE = regexp.MustCompile(`/{2,}`)
)

const canonicalScanWindow = 4096

func isTrackingParam(name string) bool {
	lower := strings.ToLower(name)
	if trackingParams[lower] {
		return true
	}
	for _, p := range trackingPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func CanonicalizeURL(rawURL string) (string, error) {
	if rawURL == "" {
		return rawURL, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, err
	}

	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawFragment = ""

	host := u.Hostname()
	port := u.Port()
	if port != "" {
		scheme := strings.ToLower(u.Scheme)
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			u.Host = host
		}
	}

	if u.RawQuery != "" {
		q := u.Query()
		keep := make([]string, 0, len(q))
		for k := range q {
			if isTrackingParam(k) {
				continue
			}
			keep = append(keep, k)
		}
		if len(keep) == 0 {
			u.RawQuery = ""
		} else {
			sort.Strings(keep)
			filtered := url.Values{}
			for _, k := range keep {
				filtered[k] = q[k]
			}
			u.RawQuery = filtered.Encode()
		}
	}

	if u.Path != "" {
		leading := ""
		p := u.Path
		if strings.HasPrefix(p, "/") {
			leading = "/"
			p = strings.TrimLeft(p, "/")
		}
		p = multiSlashRE.ReplaceAllString(p, "/")
		u.Path = leading + p
		u.RawPath = ""
	}

	return u.String(), nil
}

func ResolveCanonicalLink(htmlStr string, baseURL string) string {
	if htmlStr == "" {
		return ""
	}
	head := htmlStr
	if len(head) > canonicalScanWindow {
		head = head[:canonicalScanWindow]
	}

	var href string
	if m := canonicalLinkRelFirstRE.FindStringSubmatch(head); len(m) > 1 {
		href = strings.TrimSpace(m[1])
	} else if m := canonicalLinkHrefFirstRE.FindStringSubmatch(head); len(m) > 1 {
		href = strings.TrimSpace(m[1])
	}
	if href == "" {
		return ""
	}

	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}

	if ref.IsAbs() {
		scheme := strings.ToLower(ref.Scheme)
		if scheme != "http" && scheme != "https" {
			return ""
		}
		return ref.String()
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(ref)
	scheme := strings.ToLower(resolved.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return resolved.String()
}

func PickCanonical(rawURL, html string) string {
	if linked := ResolveCanonicalLink(html, rawURL); linked != "" {
		return linked
	}
	c, err := CanonicalizeURL(rawURL)
	if err != nil || c == rawURL {
		return ""
	}
	return c
}
