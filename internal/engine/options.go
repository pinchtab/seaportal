package engine

import (
	"context"
	"net/http"
	"time"
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
	FailFast             bool
	FastMode             bool
	ProbeSearch          bool
	NoPooling            bool
	MaxRetries           int
	MaxRetryWait         time.Duration
	TotalRetryTimeout    time.Duration
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
