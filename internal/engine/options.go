package engine

import (
	"context"
	"net/http"
	"time"
)

const (
	DefaultClientTimeout = 30 * time.Second

	DefaultMaxRetryWait = 60 * time.Second

	DefaultTotalRetryTimeout = 120 * time.Second

	DefaultRetryBackoffBase = time.Second

	DefaultSitemapMaxDepth = 5
	DefaultSitemapMaxURLs  = 50_000

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

	ClientTimeout time.Duration

	RetryBackoffBase time.Duration

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

	Transport http.RoundTripper

	Security *SecurityPolicy

	Context context.Context
}
