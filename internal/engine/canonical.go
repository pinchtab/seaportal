package engine

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// trackingParams is a curated, host-agnostic denylist of query-string parameters
// that identify ad/analytics/referral tracking rather than the resource itself.
// Keys are stored lowercased; lookups must lowercase the incoming param name.
var trackingParams = map[string]bool{
	// Google Analytics / UTM
	"utm_source":          true,
	"utm_medium":          true,
	"utm_campaign":        true,
	"utm_term":            true,
	"utm_content":         true,
	"utm_id":              true,
	"utm_brand":           true,
	"utm_creative_format": true,

	// Click identifiers
	"fbclid":  true,
	"gclid":   true,
	"dclid":   true,
	"gbraid":  true,
	"wbraid":  true,
	"msclkid": true,
	"yclid":   true,

	// GA / GTM / HubSpot / Mailchimp / Marketo
	"_ga":     true,
	"_gl":     true,
	"_hsenc":  true,
	"_hsmi":   true,
	"mkt_tok": true,
	"mc_eid":  true,
	"mc_cid":  true,

	// Social / share / referral
	"igshid":  true,
	"ref":     true,
	"ref_src": true,
	"ref_url": true,
	"s":       true, // Twitter share param
	"t":       true, // Twitter timestamp param
	"feature": true,

	// Piwik / generic campaign
	"pk_campaign": true,
	"pk_kwd":      true,
	"cmpid":       true,
	"wt.mc_id":    true,
}

// trackingPrefixes drops any param whose lowercased name starts with one of
// these prefixes — catches suffixed variants like _ga_ABC123, utm_brand_xxx.
var trackingPrefixes = []string{"_ga", "_gid", "_fb", "_hs", "utm_"}

// canonicalLinkRE pulls the href out of a <link rel="canonical"> tag.
// Case-insensitive on tag/attribute names; tolerates attribute reordering.
var (
	canonicalLinkRelFirstRE  = regexp.MustCompile(`(?is)<link\b[^>]*\brel\s*=\s*["']?canonical["']?[^>]*\bhref\s*=\s*["']([^"'>\s]+)["']?[^>]*>`)
	canonicalLinkHrefFirstRE = regexp.MustCompile(`(?is)<link\b[^>]*\bhref\s*=\s*["']([^"'>\s]+)["'][^>]*\brel\s*=\s*["']?canonical["']?[^>]*>`)

	multiSlashRE = regexp.MustCompile(`/{2,}`)
)

const canonicalScanWindow = 4096

// isTrackingParam reports whether a query parameter name matches the
// host-agnostic tracking denylist or any tracking prefix (case-insensitive).
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

// CanonicalizeURL normalises a URL by lowercasing the host, dropping the
// fragment, removing default ports, stripping tracking parameters, sorting
// remaining params, and collapsing duplicate path slashes. Malformed input is
// returned unchanged alongside the parse error — never crashes.
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

	// Strip default ports.
	host := u.Hostname()
	port := u.Port()
	if port != "" {
		scheme := strings.ToLower(u.Scheme)
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			u.Host = host
		}
	}

	// Filter + sort query.
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

	// Collapse duplicate slashes in path while preserving the single leading /.
	if u.Path != "" {
		leading := ""
		p := u.Path
		if strings.HasPrefix(p, "/") {
			leading = "/"
			p = strings.TrimLeft(p, "/")
		}
		p = multiSlashRE.ReplaceAllString(p, "/")
		u.Path = leading + p
		u.RawPath = "" // force re-encoding from Path
	}

	return u.String(), nil
}

// ResolveCanonicalLink finds <link rel="canonical" href="…"> in the first 4 KB
// of HTML and resolves the href against baseURL. Returns "" if absent or if the
// href uses a non-http(s) scheme.
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

	// If absolute, validate scheme. If relative, resolve against base.
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

// PickCanonical orchestrates canonical-URL selection. A <link rel="canonical">
// in the HTML wins; otherwise we fall back to algorithmic canonicalisation.
// Returns "" when the canonical is empty or equal to the raw URL.
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
