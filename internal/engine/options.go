package engine

import (
	"context"
	"net/http"
	"time"
)

// Engine-wide fetch defaults (T19). Each constant is applied in exactly ONE
// defaulting site — named in its comment — when the corresponding option is
// zero; nothing else may restate the literal. Exported so the CLI, MCP layer,
// and embedders reference these values instead of re-declaring them.
//
// Note: the CLI's --max-retry-wait / --retry-timeout flag defaults (30s / 90s,
// cmd/seaportal/extract.go) are deliberately tighter than the library
// defaults below — an interactive invocation should give up sooner than an
// embedded library caller. That divergence is intentional and documented at
// the flag definitions.
const (
	// DefaultClientTimeout bounds a whole HTTP exchange (dial, TLS, headers,
	// body read) when Options.ClientTimeout is zero. Defaulting site:
	// buildFetchClient (fetch_setup.go); the shared uTLS client uses the same
	// value at construction (utls.go).
	DefaultClientTimeout = 30 * time.Second

	// DefaultMaxRetryWait caps a single retry backoff wait when
	// Options.MaxRetryWait is zero. Defaulting site: resolveRetryConfig
	// (fetch_document.go).
	DefaultMaxRetryWait = 60 * time.Second

	// DefaultTotalRetryTimeout caps cumulative retry waiting when
	// Options.TotalRetryTimeout is zero. Defaulting site: resolveRetryConfig.
	DefaultTotalRetryTimeout = 120 * time.Second

	// DefaultRetryBackoffBase is the unit of exponential retry backoff (hop N
	// waits 2^N × base, capped by the max retry wait) when
	// Options.RetryBackoffBase is zero. Defaulting site: resolveRetryConfig.
	DefaultRetryBackoffBase = time.Second

	// DefaultSitemapMaxDepth / DefaultSitemapMaxURLs bound sitemap-index
	// recursion and total flattened URLs when the FlattenSitemapOptions
	// fields are zero. Defaulting site: FlattenSitemap (sitemap.go).
	DefaultSitemapMaxDepth = 5
	DefaultSitemapMaxURLs  = 50_000

	// DefaultFeedMaxItems caps parsed feed entries when
	// ParseFeedOptions.MaxItems is zero. Defaulting site: ParseFeed (feed.go).
	DefaultFeedMaxItems = 200
)

type RetryEvent struct {
	Attempt    int
	StatusCode int
	WaitTime   time.Duration
	Error      error
	Outcome    string
}

type DomainRetry struct {
	MaxRetries   int
	MaxRetryWait time.Duration
}

type Options struct {
	FailFast          bool
	FastMode          bool
	ProbeSearch       bool
	NoPooling         bool
	MaxRetries        int
	MaxRetryWait      time.Duration
	TotalRetryTimeout time.Duration

	// ClientTimeout bounds each HTTP request end-to-end (dial, TLS, response
	// headers, body read). 0 = DefaultClientTimeout. A per-domain
	// DomainTimeout entry overrides it for that domain. Replaces the
	// previously baked-in 30s client timeout (T19).
	ClientTimeout time.Duration

	HeadPreflight        bool
	ContentTypePreflight bool
	RetryLogger          func(event RetryEvent)
	DomainRetryConfig    map[string]DomainRetry
	UserAgent            string
	DomainUserAgent      map[string]string
	DomainTimeout        map[string]time.Duration
	RespectCrawlDelay    bool
	RespectRobots        bool
	CrawlDelayCache      *CrawlDelayCache
	RateLimit            time.Duration
	RateLimiter          *HostRateLimiter
	RequestID            string
	SendRequestID        bool
	Dedupe               bool
	NoNearDedupe         bool
	WithLinks            bool
	WithImages           bool
	WithTables           bool
	WithComments         bool
	Citations            bool
	LinkRetention        LinkRetention
	Chunk                ChunkConfig
	SelectCSS            string
	StripCSS             string
	MaxTokens            int
	HeadOnly             bool
	NoPruneFallback      bool
	Proxy                string
	CacheDir             string
	CacheTTL             time.Duration
	CacheStaleTolerance  time.Duration
	NoCache              bool
	NoPDF                bool
	SchemaPath           string
	Schema               *Schema
	Query                string
	TopN                 int
	FilterByQuery        bool
	SplitOut             string
	SplitBytes           int

	// Transport overrides the default utls Chrome-fingerprint transport when
	// non-nil. Primary use: tests injecting a record/replay RoundTripper from
	// internal/engine/mock so HTTP-touching tests stay hermetic. Production
	// callers should leave this nil; opts.Proxy is independently honoured.
	Transport http.RoundTripper

	// Security, when non-nil, enforces an SSRF / private-IP / redirect /
	// decompression policy across the whole fetch path. Nil (the zero value)
	// keeps the historical unguarded behaviour. Build a safe default with
	// DefaultSecurityPolicy. Safe to share across concurrent calls.
	Security *SecurityPolicy

	// Context, when non-nil, is the cancellation context for the fetch: it
	// bounds the HTTP request and makes retry backoff / crawl-delay waits
	// interruptible, so an overall deadline or SIGINT can preempt an in-flight
	// retry (ALP-043). Nil defaults to context.Background().
	//
	// Deprecated: pass the context to FromURLContext instead of embedding it
	// in Options (T14). Still honoured by FromURLWithOptions for
	// compatibility; FromURLContext overrides it with its ctx argument.
	Context context.Context
}
