package engine

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// robotsFetchTimeout bounds a single robots.txt fetch when the cache's fetch
// template does not set its own timeout (the historical 5s budget).
const robotsFetchTimeout = 5 * time.Second

// robotsMaxBody caps a robots.txt response body when the caller supplied no
// SecurityPolicy of its own. RFC 9309 requires parsers to handle at least
// 500 KiB; a body beyond this cap fails the fetch and the domain fails open —
// the same contract as an unreachable robots.txt.
const robotsMaxBody = 1 << 20

// maxCrawlDelay caps the effective robots.txt Crawl-delay at the consumption
// points (applyCrawlDelay, the scrape fetch pool): a hostile
// `Crawl-delay: 86400` must not stall every worker until the overall timeout.
// The parsed value stays truthful — GetDelay* returns it unclamped — and
// consumers surface a warning when they clamp.
const maxCrawlDelay = 30 * time.Second

// clampCrawlDelay returns the effective (possibly capped) crawl-delay and
// whether the cap was applied.
func clampCrawlDelay(d time.Duration) (time.Duration, bool) {
	if d > maxCrawlDelay {
		return maxCrawlDelay, true
	}
	return d, false
}

// CrawlDelayCache stores per-domain crawl-delay values + Allow/Disallow rules
// from robots.txt. Keeps its historical name even though it now caches rules
// too — the storage and fetch path is shared.
//
// Fetches happen outside the lock with per-domain singleflight: concurrent
// callers for one domain wait on the single in-flight fetch, and callers for
// other domains are never blocked by it.
type CrawlDelayCache struct {
	mu       sync.Mutex
	entries  map[string]robotsEntry
	inflight map[string]*robotsFetch
	// fetch is the template for robots.txt fetches (client/proxy/security);
	// the per-call pieces (context, User-Agent) are filled in fetchRobots.
	fetch FetchBytesOptions
}

// robotsFetch tracks one in-flight robots.txt fetch. entry is published
// before done is closed, so waiters may read it after <-done.
type robotsFetch struct {
	done  chan struct{}
	entry robotsEntry
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

// NewCrawlDelayCache creates a new crawl-delay + robots-rule cache that
// fetches robots.txt through the shared engine client.
func NewCrawlDelayCache() *CrawlDelayCache {
	return newCrawlDelayCacheWithFetch(FetchBytesOptions{})
}

// newCrawlDelayCacheWithFetch creates a cache whose robots.txt fetches use
// the given template (security policy, proxy client, test transport).
func newCrawlDelayCacheWithFetch(fetch FetchBytesOptions) *CrawlDelayCache {
	return &CrawlDelayCache{
		entries:  make(map[string]robotsEntry),
		inflight: make(map[string]*robotsFetch),
		fetch:    fetch,
	}
}

// GetDelay returns the crawl-delay for a domain over HTTPS.
func (c *CrawlDelayCache) GetDelay(ctx context.Context, domain string, userAgent string) time.Duration {
	return c.GetDelayWithScheme(ctx, domain, userAgent, "https")
}

// GetDelayWithScheme returns the crawl-delay for a domain using the given scheme.
func (c *CrawlDelayCache) GetDelayWithScheme(ctx context.Context, domain string, userAgent string, scheme string) time.Duration {
	entry := c.getOrFetch(ctx, domain, userAgent, scheme)
	return pickDelay(entry.delays, userAgent)
}

// IsAllowed returns true if the given path is permitted for userAgent under
// the domain's robots.txt rules. Fail-open: when robots.txt is unreachable
// (network error, 4xx, 5xx, parse failure) the path is treated as allowed.
func (c *CrawlDelayCache) IsAllowed(ctx context.Context, domain string, userAgent string, scheme string, path string) bool {
	if domain == "" {
		return true
	}
	if path == "" {
		path = "/"
	}
	entry := c.getOrFetch(ctx, domain, userAgent, scheme)
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
// existing entry is missing or expired. Always returns a usable entry (an
// empty fail-open one on any error). The network fetch runs OUTSIDE the lock:
// the first caller for a domain registers an in-flight marker and fetches;
// concurrent callers for the same domain wait on it (or bail fail-open when
// their ctx fires first); callers for other domains proceed unimpeded.
func (c *CrawlDelayCache) getOrFetch(ctx context.Context, domain string, userAgent string, scheme string) robotsEntry {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	if entry, ok := c.entries[domain]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.Unlock()
		return entry
	}
	if f, ok := c.inflight[domain]; ok {
		c.mu.Unlock()
		select {
		case <-f.done:
			return f.entry
		case <-ctx.Done():
			// This caller is being torn down; fail open without waiting.
			return robotsEntry{}
		}
	}
	f := &robotsFetch{done: make(chan struct{})}
	c.inflight[domain] = f
	c.mu.Unlock()

	entry, cacheable := c.fetchRobots(ctx, domain, userAgent, scheme)

	c.mu.Lock()
	if cacheable {
		c.entries[domain] = entry
	}
	delete(c.inflight, domain)
	c.mu.Unlock()

	f.entry = entry
	close(f.done)
	return entry
}

// fetchRobots fetches and parses robots.txt for domain through the shared
// FetchBytes path (uTLS client by default; the cache's fetch template carries
// any proxy client, security policy, or test transport). Errors and non-200s
// yield an empty fail-open entry with a 1h TTL; a fetch that failed only
// because the caller's ctx fired is not cached at all, so one cancelled run
// can't poison a shared cache.
func (c *CrawlDelayCache) fetchRobots(ctx context.Context, domain string, userAgent string, scheme string) (entry robotsEntry, cacheable bool) {
	if scheme == "" {
		scheme = "https"
	}
	fo := c.fetch
	if fo.Timeout <= 0 {
		fo.Timeout = robotsFetchTimeout
	}
	fo.UserAgent = userAgent
	if fo.Security == nil && fo.Client == nil {
		// No caller policy: still bound the body read and redirect chain.
		fo.Security = &SecurityPolicy{
			MaxRedirects:         10,
			MaxResponseBytes:     robotsMaxBody,
			MaxDecompressedBytes: robotsMaxBody,
		}
	}

	now := time.Now()
	empty := robotsEntry{
		delays:    map[string]time.Duration{},
		rules:     map[string][]compiledRule{},
		lastFetch: now,
		expiresAt: now.Add(1 * time.Hour),
	}

	body, _, status, err := FetchBytes(ctx, fmt.Sprintf("%s://%s/robots.txt", scheme, domain), fo)
	if err != nil {
		if ctx.Err() != nil {
			return robotsEntry{}, false
		}
		return empty, true
	}
	if status != http.StatusOK {
		return empty, true
	}

	delays, rawRules := parseRobotsTxt(string(body))
	return robotsEntry{
		delays:    delays,
		rules:     compileRules(rawRules),
		lastFetch: now,
		expiresAt: now.Add(24 * time.Hour),
	}, true
}

// parseRobotsTxt walks robots.txt once and returns per-UA crawl-delay values
// and per-UA Allow/Disallow rules (in source order). UA section names are
// lowercased; "*" represents the wildcard section.
func parseRobotsTxt(content string) (delays map[string]time.Duration, rules map[string][]robotsRule) {
	delays = map[string]time.Duration{}
	rules = map[string][]robotsRule{}

	lines := strings.Split(content, "\n")
	// Consecutive User-agent: lines share the directives that follow.
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

// productToken extracts the leading RFC 9309 product token from a User-Agent
// string or robots.txt section name: the longest prefix of letters, hyphens,
// and underscores, lowercased. "seaportal-test/1.0" → "seaportal-test",
// "Mozilla/5.0 (…) Chrome/122" → "mozilla".
func productToken(s string) string {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '-' || c == '_' {
			end++
			continue
		}
		break
	}
	return strings.ToLower(s[:end])
}

// matchRobotsSections returns the robots.txt section names binding userAgent
// per RFC 9309 longest-match token-prefix semantics: a section applies when
// its product token is a prefix of the crawler's product token (`googlebot`
// binds a `googlebot-news` crawler, but `bot` does not bind a Chrome UA), and
// only the section(s) carrying the longest matching token win. Sections that
// share the winning token are all returned, sorted, because the RFC merges
// same-agent groups. The "*" wildcard is the caller's fallback, never
// returned here.
//
// The product token is derived from the CONFIGURED User-Agent, so
// self-identification does not leak through an impersonated UA: with the
// default Chrome UA a `User-agent: seaportal` section does NOT bind.
func matchRobotsSections[V any](sections map[string]V, userAgent string) []string {
	token := productToken(userAgent)
	if token == "" {
		return nil
	}
	bestLen := 0
	var best []string
	for agent := range sections {
		if agent == "*" {
			continue
		}
		at := productToken(agent)
		if at == "" || !strings.HasPrefix(token, at) {
			continue
		}
		switch {
		case len(at) > bestLen:
			bestLen = len(at)
			best = append(best[:0], agent)
		case len(at) == bestLen:
			best = append(best, agent)
		}
	}
	sort.Strings(best)
	return best
}

// pickRules returns the rule set for the user-agent section(s) matching
// userAgent per RFC 9309, falling back to the "*" wildcard section.
func pickRules(rules map[string][]compiledRule, userAgent string) []compiledRule {
	if len(rules) == 0 {
		return nil
	}
	matched := matchRobotsSections(rules, userAgent)
	if len(matched) == 1 {
		return rules[matched[0]]
	}
	if len(matched) > 1 {
		var combined []compiledRule
		for _, agent := range matched {
			combined = append(combined, rules[agent]...)
		}
		return combined
	}
	return rules["*"]
}

// pickDelay returns the crawl-delay for the user-agent section(s) matching
// userAgent per RFC 9309, falling back to the "*" section. When several
// sections share the winning token the largest (most polite) delay applies.
// Returns 0 if none.
func pickDelay(delays map[string]time.Duration, userAgent string) time.Duration {
	if len(delays) == 0 {
		return 0
	}
	matched := matchRobotsSections(delays, userAgent)
	if len(matched) > 0 {
		var max time.Duration
		for _, agent := range matched {
			if d := delays[agent]; d > max {
				max = d
			}
		}
		return max
	}
	return delays["*"]
}
