// Package portal provides content extraction with SPA detection
package engine

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CrawlDelayCache stores per-domain crawl-delay values + Allow/Disallow rules
// from robots.txt. Keeps its historical name even though it now caches rules
// too — the storage and fetch path is shared.
type CrawlDelayCache struct {
	mu      sync.RWMutex
	delays  map[string]robotsEntry
	fetched map[string]time.Time // When we last fetched robots.txt for each domain
}

type robotsEntry struct {
	delays    map[string]time.Duration  // per-UA crawl-delay (lowercased UA section; "*" for wildcard)
	rules     map[string][]compiledRule // per-UA Allow/Disallow rules in source order
	lastFetch time.Time
	expiresAt time.Time
}

// robotsRule is the parsed (un-compiled) form harvested from robots.txt.
type robotsRule struct {
	Allow   bool
	Pattern string
}

// compiledRule augments robotsRule with the pre-compiled wildcard regex.
type compiledRule struct {
	Allow   bool
	Pattern string
	re      *regexp.Regexp // nil means literal-prefix match
}

// NewCrawlDelayCache creates a new crawl-delay + robots-rule cache.
func NewCrawlDelayCache() *CrawlDelayCache {
	return &CrawlDelayCache{
		delays:  make(map[string]robotsEntry),
		fetched: make(map[string]time.Time),
	}
}

// GetDelay returns the crawl-delay for a domain over HTTPS.
func (c *CrawlDelayCache) GetDelay(domain string, userAgent string) time.Duration {
	return c.GetDelayWithScheme(domain, userAgent, "https")
}

// GetDelayWithScheme returns the crawl-delay for a domain using the given scheme.
func (c *CrawlDelayCache) GetDelayWithScheme(domain string, userAgent string, scheme string) time.Duration {
	entry := c.getOrFetch(domain, userAgent, scheme)
	return pickDelay(entry.delays, userAgent)
}

// IsAllowed returns true if the given path is permitted for userAgent under
// the domain's robots.txt rules. Fail-open: when robots.txt is unreachable
// (network error, 4xx, 5xx, parse failure) the path is treated as allowed.
func (c *CrawlDelayCache) IsAllowed(domain string, userAgent string, scheme string, path string) bool {
	if domain == "" {
		return true
	}
	if path == "" {
		path = "/"
	}
	entry := c.getOrFetch(domain, userAgent, scheme)
	rules := pickRules(entry.rules, userAgent)
	if len(rules) == 0 {
		return true
	}

	bestLen := -1
	bestAllow := true
	for _, r := range rules {
		if !ruleMatches(r, path) {
			continue
		}
		l := len(r.Pattern)
		switch {
		case l > bestLen:
			bestLen = l
			bestAllow = r.Allow
		case l == bestLen && r.Allow:
			// Allow beats Disallow on tie.
			bestAllow = true
		}
	}
	if bestLen < 0 {
		return true
	}
	return bestAllow
}

// getOrFetch returns the cache entry for domain, fetching robots.txt if the
// existing entry is missing or expired. Always returns a non-nil entry (an
// empty one on any error) so callers can use it without nil checks.
func (c *CrawlDelayCache) getOrFetch(domain string, userAgent string, scheme string) robotsEntry {
	c.mu.RLock()
	entry, ok := c.delays[domain]
	c.mu.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry
	}
	return c.fetchAndCache(domain, userAgent, scheme)
}

// fetchAndCache fetches robots.txt and caches both crawl-delay and rules.
func (c *CrawlDelayCache) fetchAndCache(domain string, userAgent string, scheme string) robotsEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock.
	if entry, ok := c.delays[domain]; ok && time.Now().Before(entry.expiresAt) {
		return entry
	}

	emptyEntry := func(ttl time.Duration) robotsEntry {
		return robotsEntry{
			delays:    map[string]time.Duration{},
			rules:     map[string][]compiledRule{},
			lastFetch: time.Now(),
			expiresAt: time.Now().Add(ttl),
		}
	}

	if scheme == "" {
		scheme = "https"
	}
	robotsURL := fmt.Sprintf("%s://%s/robots.txt", scheme, domain)
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", robotsURL, nil)
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		entry := emptyEntry(1 * time.Hour)
		c.delays[domain] = entry
		return entry
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		entry := emptyEntry(1 * time.Hour)
		c.delays[domain] = entry
		return entry
	}

	// Cap at 512 KB — some robots.txt files are absurdly large.
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		entry := emptyEntry(1 * time.Hour)
		c.delays[domain] = entry
		return entry
	}

	delays, rawRules := parseRobotsTxt(string(bodyBytes))
	compiled := compileRules(rawRules)
	entry := robotsEntry{
		delays:    delays,
		rules:     compiled,
		lastFetch: time.Now(),
		expiresAt: time.Now().Add(24 * time.Hour),
	}
	c.delays[domain] = entry
	return entry
}

// parseRobotsTxt walks robots.txt once and returns per-UA crawl-delay values
// and per-UA Allow/Disallow rules (in source order). UA section names are
// lowercased; "*" represents the wildcard section.
func parseRobotsTxt(content string) (delays map[string]time.Duration, rules map[string][]robotsRule) {
	delays = map[string]time.Duration{}
	rules = map[string][]robotsRule{}

	lines := strings.Split(content, "\n")
	// Group consecutive User-agent: lines so they all share the directives that follow.
	var currentAgents []string
	expectingAgents := true

	flush := func(directive string) (string, string, bool) {
		idx := strings.Index(directive, ":")
		if idx < 0 {
			return "", "", false
		}
		key := strings.ToLower(strings.TrimSpace(directive[:idx]))
		val := strings.TrimSpace(directive[idx+1:])
		// Strip trailing inline comment.
		if h := strings.Index(val, "#"); h >= 0 {
			val = strings.TrimSpace(val[:h])
		}
		return key, val, true
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := flush(line)
		if !ok {
			continue
		}

		switch key {
		case "user-agent":
			if !expectingAgents {
				currentAgents = currentAgents[:0]
				expectingAgents = true
			}
			currentAgents = append(currentAgents, strings.ToLower(val))
		case "crawl-delay":
			expectingAgents = false
			if d, err := time.ParseDuration(val + "s"); err == nil && d > 0 {
				for _, a := range currentAgents {
					if _, exists := delays[a]; !exists {
						delays[a] = d
					}
				}
			}
		case "allow", "disallow":
			expectingAgents = false
			// Empty Allow is a no-op per the spec; empty Disallow is an explicit allow-all.
			isAllow := key == "allow"
			if isAllow && val == "" {
				continue
			}
			rule := robotsRule{Allow: isAllow, Pattern: val}
			for _, a := range currentAgents {
				rules[a] = append(rules[a], rule)
			}
		default:
			// Sitemap, host, etc. — ignored.
			expectingAgents = false
		}
	}
	return delays, rules
}

// compileRules pre-compiles each rule pattern into a regex (when wildcards are
// present) for cheaper repeated matching.
func compileRules(raw map[string][]robotsRule) map[string][]compiledRule {
	out := make(map[string][]compiledRule, len(raw))
	for agent, rs := range raw {
		compiled := make([]compiledRule, 0, len(rs))
		for _, r := range rs {
			cr := compiledRule{Allow: r.Allow, Pattern: r.Pattern}
			if strings.ContainsAny(r.Pattern, "*$") {
				cr.re = compileRobotsPattern(r.Pattern)
			}
			compiled = append(compiled, cr)
		}
		out[agent] = compiled
	}
	return out
}

// compileRobotsPattern translates a robots.txt pattern into an anchored regex.
// `*` matches any sequence of characters; `$` at the end anchors to end-of-path.
func compileRobotsPattern(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			b.WriteString(".*")
		case '$':
			if i == len(pattern)-1 {
				b.WriteByte('$')
			} else {
				b.WriteString(regexp.QuoteMeta("$"))
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil
	}
	return re
}

// ruleMatches reports whether the rule applies to the given path. Empty
// Disallow patterns explicitly match nothing (= allow-all, but they never
// participate in longest-match scoring because we return false here).
func ruleMatches(r compiledRule, path string) bool {
	if r.Pattern == "" {
		return false
	}
	if r.re != nil {
		return r.re.MatchString(path)
	}
	return strings.HasPrefix(path, r.Pattern)
}

// pickRules returns the rule set for the most-specific user-agent section
// matching userAgent, falling back to the "*" wildcard section.
func pickRules(rules map[string][]compiledRule, userAgent string) []compiledRule {
	if len(rules) == 0 {
		return nil
	}
	uaLower := strings.ToLower(userAgent)

	var bestAgent string
	for agent := range rules {
		if agent == "*" {
			continue
		}
		if strings.Contains(uaLower, agent) || strings.Contains(agent, "seaportal") {
			if len(agent) > len(bestAgent) {
				bestAgent = agent
			}
		}
	}
	if bestAgent != "" {
		return rules[bestAgent]
	}
	if wild, ok := rules["*"]; ok {
		return wild
	}
	return nil
}

// pickDelay returns the crawl-delay for the most-specific UA section matching
// userAgent, falling back to the "*" section. Returns 0 if none.
func pickDelay(delays map[string]time.Duration, userAgent string) time.Duration {
	if len(delays) == 0 {
		return 0
	}
	uaLower := strings.ToLower(userAgent)
	var bestAgent string
	for agent := range delays {
		if agent == "*" {
			continue
		}
		if strings.Contains(uaLower, agent) || strings.Contains(agent, "seaportal") {
			if len(agent) > len(bestAgent) {
				bestAgent = agent
			}
		}
	}
	if bestAgent != "" {
		return delays[bestAgent]
	}
	return delays["*"]
}
