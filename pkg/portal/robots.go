// Package portal provides content extraction with SPA detection
package portal

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CrawlDelayCache stores per-domain crawl-delay values from robots.txt
type CrawlDelayCache struct {
	mu      sync.RWMutex
	delays  map[string]crawlDelayEntry
	fetched map[string]time.Time // When we last fetched robots.txt for each domain
}

type crawlDelayEntry struct {
	delay     time.Duration
	lastFetch time.Time
	expiresAt time.Time
}

// NewCrawlDelayCache creates a new crawl-delay cache
func NewCrawlDelayCache() *CrawlDelayCache {
	return &CrawlDelayCache{
		delays:  make(map[string]crawlDelayEntry),
		fetched: make(map[string]time.Time),
	}
}

// GetDelay returns the crawl-delay for a domain, fetching robots.txt if needed.
// Returns 0 if no crawl-delay is set or if robots.txt is unavailable.
// The scheme parameter (http or https) determines which protocol to use for fetching robots.txt.
func (c *CrawlDelayCache) GetDelay(domain string, userAgent string) time.Duration {
	return c.GetDelayWithScheme(domain, userAgent, "https")
}

// GetDelayWithScheme returns the crawl-delay for a domain using the specified scheme.
func (c *CrawlDelayCache) GetDelayWithScheme(domain string, userAgent string, scheme string) time.Duration {
	c.mu.RLock()
	entry, ok := c.delays[domain]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		return entry.delay
	}

	// Need to fetch or refresh robots.txt
	return c.fetchAndCacheDelay(domain, userAgent, scheme)
}

// fetchAndCacheDelay fetches robots.txt and extracts Crawl-delay
func (c *CrawlDelayCache) fetchAndCacheDelay(domain string, userAgent string, scheme string) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if entry, ok := c.delays[domain]; ok && time.Now().Before(entry.expiresAt) {
		return entry.delay
	}

	// Fetch robots.txt with a short timeout
	robotsURL := fmt.Sprintf("%s://%s/robots.txt", scheme, domain)
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", robotsURL, nil)
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		// Cache as "no delay" for 1 hour to avoid repeated failed fetches
		c.delays[domain] = crawlDelayEntry{
			delay:     0,
			lastFetch: time.Now(),
			expiresAt: time.Now().Add(1 * time.Hour),
		}
		return 0
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		c.delays[domain] = crawlDelayEntry{
			delay:     0,
			lastFetch: time.Now(),
			expiresAt: time.Now().Add(1 * time.Hour),
		}
		return 0
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)) // Limit to 64KB
	if err != nil {
		c.delays[domain] = crawlDelayEntry{
			delay:     0,
			lastFetch: time.Now(),
			expiresAt: time.Now().Add(1 * time.Hour),
		}
		return 0
	}

	delay := parseRobotsCrawlDelay(string(bodyBytes), userAgent)
	c.delays[domain] = crawlDelayEntry{
		delay:     delay,
		lastFetch: time.Now(),
		expiresAt: time.Now().Add(24 * time.Hour), // Cache for 24 hours
	}
	return delay
}

// parseRobotsCrawlDelay extracts Crawl-delay from robots.txt content.
// Looks for User-agent: * or matching user-agent sections.
func parseRobotsCrawlDelay(content string, userAgent string) time.Duration {
	lines := strings.Split(content, "\n")
	inMatchingSection := false
	inWildcardSection := false
	var wildcardDelay time.Duration
	var specificDelay time.Duration

	// Normalize user-agent for comparison
	uaLower := strings.ToLower(userAgent)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for User-agent directive
		if strings.HasPrefix(strings.ToLower(line), "user-agent:") {
			agentPart := strings.TrimSpace(line[11:])
			agentLower := strings.ToLower(agentPart)

			if agentLower == "*" { //nolint:gocritic
				inWildcardSection = true
				inMatchingSection = false
			} else if strings.Contains(uaLower, agentLower) || strings.Contains(agentLower, "seaportal") {
				inMatchingSection = true
				inWildcardSection = false
			} else {
				inMatchingSection = false
				inWildcardSection = false
			}
			continue
		}

		// Check for Crawl-delay directive
		if strings.HasPrefix(strings.ToLower(line), "crawl-delay:") {
			delayStr := strings.TrimSpace(line[12:])
			if delay, err := time.ParseDuration(delayStr + "s"); err == nil {
				if inMatchingSection {
					specificDelay = delay
				} else if inWildcardSection {
					wildcardDelay = delay
				}
			}
		}
	}

	// Specific user-agent match takes precedence over wildcard
	if specificDelay > 0 {
		return specificDelay
	}
	return wildcardDelay
}
