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

const robotsFetchTimeout = 5 * time.Second

const robotsMaxBody = 1 << 20

const maxCrawlDelay = 30 * time.Second

func clampCrawlDelay(d time.Duration) (time.Duration, bool) {
	if d > maxCrawlDelay {
		return maxCrawlDelay, true
	}
	return d, false
}

type CrawlDelayCache struct {
	mu       sync.Mutex
	entries  map[string]robotsEntry
	inflight map[string]*robotsFetch
	fetch    FetchBytesOptions
}

type robotsFetch struct {
	done  chan struct{}
	entry robotsEntry
}

type robotsEntry struct {
	delays    map[string]time.Duration
	rules     map[string][]compiledRule
	lastFetch time.Time
	expiresAt time.Time
}

type robotsRule struct {
	Allow   bool
	Pattern string
}

type compiledRule struct {
	Allow   bool
	Pattern string
	re      *regexp.Regexp
}

func NewCrawlDelayCache() *CrawlDelayCache {
	return newCrawlDelayCacheWithFetch(FetchBytesOptions{})
}

func newCrawlDelayCacheWithFetch(fetch FetchBytesOptions) *CrawlDelayCache {
	return &CrawlDelayCache{
		entries:  make(map[string]robotsEntry),
		inflight: make(map[string]*robotsFetch),
		fetch:    fetch,
	}
}

func (c *CrawlDelayCache) GetDelay(ctx context.Context, domain string, userAgent string) time.Duration {
	return c.GetDelayWithScheme(ctx, domain, userAgent, "https")
}

func (c *CrawlDelayCache) GetDelayWithScheme(ctx context.Context, domain string, userAgent string, scheme string) time.Duration {
	entry := c.getOrFetch(ctx, domain, userAgent, scheme)
	return pickDelay(entry.delays, userAgent)
}

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
			bestAllow = true
		}
	}
	if bestLen < 0 {
		return true
	}
	return bestAllow
}

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

func parseRobotsTxt(content string) (delays map[string]time.Duration, rules map[string][]robotsRule) {
	delays = map[string]time.Duration{}
	rules = map[string][]robotsRule{}

	lines := strings.Split(content, "\n")
	var currentAgents []string
	expectingAgents := true

	flush := func(directive string) (string, string, bool) {
		idx := strings.Index(directive, ":")
		if idx < 0 {
			return "", "", false
		}
		key := strings.ToLower(strings.TrimSpace(directive[:idx]))
		val := strings.TrimSpace(directive[idx+1:])
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
			isAllow := key == "allow"
			if isAllow && val == "" {
				continue
			}
			rule := robotsRule{Allow: isAllow, Pattern: val}
			for _, a := range currentAgents {
				rules[a] = append(rules[a], rule)
			}
		default:
			expectingAgents = false
		}
	}
	return delays, rules
}

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

func ruleMatches(r compiledRule, path string) bool {
	if r.Pattern == "" {
		return false
	}
	if r.re != nil {
		return r.re.MatchString(path)
	}
	return strings.HasPrefix(path, r.Pattern)
}

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
