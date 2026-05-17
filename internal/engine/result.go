package engine

import (
	"net/http"
	"time"
)

type Result struct {
	URL              string   `json:"url"`
	CanonicalURL     string   `json:"canonicalUrl,omitempty"`
	Title            string   `json:"title"`
	Content          string   `json:"content"`
	Byline           string   `json:"byline"`
	PublishedDate    string   `json:"publishedDate,omitempty"`
	Language         string   `json:"language,omitempty"`
	Charset          string   `json:"charset,omitempty"`
	Section          string   `json:"section,omitempty"`
	Description      string   `json:"description,omitempty"`
	ImageURL         string   `json:"imageUrl,omitempty"`
	Excerpt          string   `json:"excerpt"`
	SiteName         string   `json:"sitename"`
	Length           int      `json:"length"`
	TimeMs           int64    `json:"timeMs"`
	Confidence       int      `json:"confidence"`
	IsSPA            bool     `json:"isSpa"`
	IsBlocked        bool     `json:"isBlocked"`
	SPASignals       []string `json:"spaSignals,omitempty"`
	Error            string   `json:"error,omitempty"`
	FromCache        bool     `json:"fromCache,omitempty"`
	CacheHit         bool     `json:"cacheHit,omitempty"`
	CacheRevalidated bool     `json:"cacheRevalidated,omitempty"`
	CacheStale       bool     `json:"cacheStale,omitempty"`
	StatusCode       int      `json:"statusCode,omitempty"`
	HeadOnly         bool     `json:"headOnly,omitempty"`
	Protocol         string   `json:"protocol,omitempty"`

	BlockedByRobots bool `json:"blockedByRobots,omitempty"`

	HeadingCount   int `json:"headingCount"`
	LinkCount      int `json:"linkCount"`
	ParagraphCount int `json:"paragraphCount"`

	Quality     float64        `json:"quality"`
	QualityInfo QualityMetrics `json:"qualityInfo"`
	Profile     PageProfile    `json:"profile"`
	PageClass   PageClass      `json:"pageClass"`
	Fingerprint string         `json:"fingerprint"`
	Validation  Validation     `json:"validation"`

	RetryCount          int           `json:"retryCount,omitempty"`
	TotalRetryWait      time.Duration `json:"totalRetryWait,omitempty"`
	HeadPreflightStatus int           `json:"headPreflightStatus,omitempty"`

	FetchTimeMs   int64 `json:"fetchTimeMs,omitempty"`
	ParseTimeMs   int64 `json:"parseTimeMs,omitempty"`
	ConvertTimeMs int64 `json:"convertTimeMs,omitempty"`

	ContentLength       int64  `json:"contentLength,omitempty"`
	ResponseContentType string `json:"responseContentType,omitempty"`

	RedirectCount int      `json:"redirectCount,omitempty"`
	RedirectChain []string `json:"redirectChain,omitempty"`
	FinalURL      string   `json:"finalUrl,omitempty"`

	IsSoft404    bool     `json:"isSoft404,omitempty"`
	Soft404Hints []string `json:"soft404Hints,omitempty"`

	ResponseETag         string `json:"responseEtag,omitempty"`
	ResponseLastModified string `json:"responseLastModified,omitempty"`

	ResponseContentEncoding string `json:"responseContentEncoding,omitempty"`
	ResponseServer          string `json:"responseServer,omitempty"`
	ResponseXForwardedFor   string `json:"responseXForwardedFor,omitempty"`

	RequestAcceptEncoding string `json:"requestAcceptEncoding,omitempty"`

	TTFBMs     int64 `json:"ttfbMs,omitempty"`
	DownloadMs int64 `json:"downloadMs,omitempty"`

	ResponseVia        string `json:"responseVia,omitempty"`
	ResponseConnection string `json:"responseConnection,omitempty"`
	ResponseAge        string `json:"responseAge,omitempty"`

	ResponseCacheControl string `json:"responseCacheControl,omitempty"`
	ResponseXCache       string `json:"responseXCache,omitempty"`
	ResponseVary         string `json:"responseVary,omitempty"`

	ResponseXCacheHits       string `json:"responseXCacheHits,omitempty"`
	ResponseSurrogateControl string `json:"responseSurrogateControl,omitempty"`
	ResponseCFCacheStatus    string `json:"responseCfCacheStatus,omitempty"`

	ResponseXServedBy        string `json:"responseXServedBy,omitempty"`
	ResponseXFastlyRequestID string `json:"responseXFastlyRequestId,omitempty"`

	ResponseXAkamaiTransformed string `json:"responseXAkamaiTransformed,omitempty"`
	ResponseXAkamaiSessionInfo string `json:"responseXAkamaiSessionInfo,omitempty"`
	ResponseXAkamaiRequestID   string `json:"responseXAkamaiRequestId,omitempty"`

	ResponseXRequestId     string `json:"responseXRequestId,omitempty"`
	ResponseXCorrelationId string `json:"responseXCorrelationId,omitempty"`

	ResponseXVarnish string `json:"responseXVarnish,omitempty"`

	ResponseXCDN string `json:"responseXCdn,omitempty"`

	ResponseXTraceId        string `json:"responseXTraceId,omitempty"`
	ResponseXB3TraceId      string `json:"responseXB3TraceId,omitempty"`
	ResponseXB3SpanId       string `json:"responseXB3SpanId,omitempty"`
	ResponseXB3ParentSpanId string `json:"responseXB3ParentSpanId,omitempty"`
	ResponseXB3Sampled      string `json:"responseXB3Sampled,omitempty"`
	ResponseB3              string `json:"responseB3,omitempty"`
	ResponseTraceparent     string `json:"responseTraceparent,omitempty"`
	ResponseTracestate      string `json:"responseTracestate,omitempty"`
	ResponseXAmznTraceId    string `json:"responseXAmznTraceId,omitempty"`

	TraceFormats     []string `json:"traceFormats,omitempty"`
	TraceCorrelation string   `json:"traceCorrelation,omitempty"`

	ResponseNEL string `json:"responseNel,omitempty"`

	ResponseReportTo string `json:"responseReportTo,omitempty"`

	ResponsePermissionsPolicy  string `json:"responsePermissionsPolicy,omitempty"`
	ResponseExpectCT           string `json:"responseExpectCt,omitempty"`
	ResponseFeaturePolicy      string `json:"responseFeaturePolicy,omitempty"`
	ResponseReportingEndpoints string `json:"responseReportingEndpoints,omitempty"`
	ResponseCSP                string `json:"responseCsp,omitempty"`
	ResponseCSPReportOnly      string `json:"responseCspReportOnly,omitempty"`

	ResponseCORP string `json:"responseCorp,omitempty"`
	ResponseCOEP string `json:"responseCoep,omitempty"`
	ResponseCOOP string `json:"responseCoop,omitempty"`

	ResponseHSTS string `json:"responseHsts,omitempty"`

	ResponseXContentTypeOptions string `json:"responseXContentTypeOptions,omitempty"`
	ResponseXFrameOptions       string `json:"responseXFrameOptions,omitempty"`

	ResponseReferrerPolicy string `json:"responseReferrerPolicy,omitempty"`

	ResponseXXSSProtection                string `json:"responseXXssProtection,omitempty"`
	ResponseXPermittedCrossDomainPolicies string `json:"responseXPermittedCrossDomainPolicies,omitempty"`
	ResponseXDownloadOptions              string `json:"responseXDownloadOptions,omitempty"`

	ResponseClearSiteData string `json:"responseClearSiteData,omitempty"`

	ResponseTimingAllowOrigin string `json:"responseTimingAllowOrigin,omitempty"`

	ResponseOriginAgentCluster string `json:"responseOriginAgentCluster,omitempty"`

	ResponseDocumentPolicy string `json:"responseDocumentPolicy,omitempty"`

	ResponseAcceptCH string `json:"responseAcceptCH,omitempty"`

	ResponseSecCHUA                 string `json:"responseSecChUa,omitempty"`
	ResponseSecCHUAMobile           string `json:"responseSecChUaMobile,omitempty"`
	ResponseSecCHUAPlatform         string `json:"responseSecChUaPlatform,omitempty"`
	ResponseSecCHUAFullVersionList  string `json:"responseSecChUaFullVersionList,omitempty"`
	ResponseSecCHPrefersColorScheme string `json:"responseSecChPrefersColorScheme,omitempty"`

	ResponseCriticalCH string `json:"responseCriticalCh,omitempty"`

	ResponseCOEPReportOnly string `json:"responseCoepReportOnly,omitempty"`
	ResponseCOOPReportOnly string `json:"responseCoopReportOnly,omitempty"`

	ResponseDocumentPolicyReportOnly string `json:"responseDocumentPolicyReportOnly,omitempty"`

	ResponseSourceMap string `json:"responseSourceMap,omitempty"`

	ResponseAccessControlAllowOrigin      string `json:"responseAccessControlAllowOrigin,omitempty"`
	ResponseAccessControlAllowMethods     string `json:"responseAccessControlAllowMethods,omitempty"`
	ResponseAccessControlAllowHeaders     string `json:"responseAccessControlAllowHeaders,omitempty"`
	ResponseAccessControlAllowCredentials string `json:"responseAccessControlAllowCredentials,omitempty"`
	ResponseAccessControlExposeHeaders    string `json:"responseAccessControlExposeHeaders,omitempty"`
	ResponseAccessControlMaxAge           string `json:"responseAccessControlMaxAge,omitempty"`

	ResponseLink string `json:"responseLink,omitempty"`

	LLMsTxtURL string `json:"llmsTxtUrl,omitempty"`

	LDJSONBlocks []LDJSONBlock `json:"ldJsonBlocks,omitempty"`

	HasLLMContent bool `json:"hasLlmContent,omitempty"`

	ResponseXRobotsTag string `json:"responseXRobotsTag,omitempty"`

	ResponseContentDisposition string `json:"responseContentDisposition,omitempty"`

	ResponseXContentDuration string `json:"responseXContentDuration,omitempty"`

	ResponseRefresh string `json:"responseRefresh,omitempty"` // Refresh header for HTTP-level redirect/meta refresh (e.g., "5; url=https://example.com")

	ResponseContentLanguage string `json:"responseContentLanguage,omitempty"`

	ResponseXUACompatible string `json:"responseXUaCompatible,omitempty"`

	ResponseAcceptRanges string `json:"responseAcceptRanges,omitempty"`

	ResponseTransferEncoding string `json:"responseTransferEncoding,omitempty"`

	ResponseContentRange string `json:"responseContentRange,omitempty"`

	ResponsePragma string `json:"responsePragma,omitempty"`

	ResponseXPoweredBy string `json:"responseXPoweredBy,omitempty"`

	ResponseXAspNetVersion string `json:"responseXAspNetVersion,omitempty"`

	ResponseXAspNetMvcVersion string `json:"responseXAspNetMvcVersion,omitempty"`

	ResponseServerTiming string `json:"responseServerTiming,omitempty"`

	ResponseXGenerator string `json:"responseXGenerator,omitempty"`

	ResponseXRuntime string `json:"responseXRuntime,omitempty"`

	ResponseXDrupalCache string `json:"responseXDrupalCache,omitempty"`

	ResponseXMagentoCacheControl string `json:"responseXMagentoCacheControl,omitempty"`

	ResponseXDrupalDynamicCache string `json:"responseXDrupalDynamicCache,omitempty"`

	ResponseXMagentoTags string `json:"responseXMagentoTags,omitempty"`

	ResponseXShopifyStage string `json:"responseXShopifyStage,omitempty"`

	ResponseXShopifyRequestID string `json:"responseXShopifyRequestId,omitempty"`

	ResponseXWPTotal      string `json:"responseXWpTotal,omitempty"`
	ResponseXWPTotalPages string `json:"responseXWpTotalPages,omitempty"`

	ResponseXCraftCache string `json:"responseXCraftCache,omitempty"`

	ResponseXDiscourseRoute string `json:"responseXDiscourseRoute,omitempty"`

	ResponseXGhostCacheStatus string `json:"responseXGhostCacheStatus,omitempty"`

	ResponseXJoomlaCache string `json:"responseXJoomlaCache,omitempty"`

	ResponseXDiscourseMediaType string `json:"responseXDiscourseMediaType,omitempty"`

	ResponseXPrestaShopCache string `json:"responseXPrestaShopCache,omitempty"`

	ResponseXMagentoCacheDebug string `json:"responseXMagentoCacheDebug,omitempty"`

	ResponseXTypo3Cache string `json:"responseXTypo3Cache,omitempty"`

	ResponseXWixRequestId string `json:"responseXWixRequestId,omitempty"`

	ResponseXSquarespaceRequestId string `json:"responseXSquarespaceRequestId,omitempty"`

	ResponseXWebflowRequestId string `json:"responseXWebflowRequestId,omitempty"`

	ResponseXContentfulRequestId string `json:"responseXContentfulRequestId,omitempty"`

	ResponseXNetlifyRequestId string `json:"responseXNetlifyRequestId,omitempty"`

	ResponseXVercelId string `json:"responseXVercelId,omitempty"`

	ResponseXHerokuRequestId string `json:"responseXHerokuRequestId,omitempty"`

	ResponseXRenderRequestId string `json:"responseXRenderRequestId,omitempty"`

	ResponseXRailwayRequestId string `json:"responseXRailwayRequestId,omitempty"`

	ResponseXFlyRequestId string `json:"responseXFlyRequestId,omitempty"`

	ResponseXDenoRegion string `json:"responseXDenoRegion,omitempty"`

	ResponseXCloudflareWorkersRequestId string `json:"responseXCloudflareWorkersRequestId,omitempty"`

	ResponseXAzureRef string `json:"responseXAzureRef,omitempty"`

	ResponseXGCPRegion string `json:"responseXGcpRegion,omitempty"`

	ResponseXAmzCfId string `json:"responseXAmzCfId,omitempty"`

	NormalizedCacheStatus string `json:"normalizedCacheStatus,omitempty"`
	CacheStatusSource     string `json:"cacheStatusSource,omitempty"`

	CDNProvider string   `json:"cdnProvider,omitempty"`
	CDNSignals  []string `json:"cdnSignals,omitempty"`

	ViaHops     []ViaHop `json:"viaHops,omitempty"`
	ProxyLayers int      `json:"proxyLayers,omitempty"`

	CacheAge       int  `json:"cacheAge,omitempty"`
	CacheMaxAge    int  `json:"cacheMaxAge,omitempty"`
	CacheSMaxAge   int  `json:"cacheSMaxAge,omitempty"`
	CacheFreshness int  `json:"cacheFreshness,omitempty"`
	CacheIsStale   bool `json:"cacheIsStale,omitempty"`

	CacheStaleWhileRevalidate int  `json:"cacheStaleWhileRevalidate,omitempty"`
	CacheStaleIfError         int  `json:"cacheStaleIfError,omitempty"`
	CacheHasSWR               bool `json:"cacheHasSwr,omitempty"`

	CDNEdgeLocation string `json:"cdnEdgeLocation,omitempty"`

	CacheMustRevalidate  bool     `json:"cacheMustRevalidate,omitempty"`
	CacheProxyRevalidate bool     `json:"cacheProxyRevalidate,omitempty"`
	CacheNoCache         bool     `json:"cacheNoCache,omitempty"`
	CacheNoCacheFields   []string `json:"cacheNoCacheFields,omitempty"`
	CacheNoStore         bool     `json:"cacheNoStore,omitempty"`
	CacheImmutable       bool     `json:"cacheImmutable,omitempty"`
	CachePrivate         bool     `json:"cachePrivate,omitempty"`
	CachePublic          bool     `json:"cachePublic,omitempty"`

	CachePolicySummary string `json:"cachePolicySummary,omitempty"`
	CachePolicyScore   int    `json:"cachePolicyScore,omitempty"`

	EffectiveCDNTTL       int      `json:"effectiveCdnTtl,omitempty"`
	CacheRecommendations  []string `json:"cacheRecommendations,omitempty"`
	CDNOptimizationIssues []string `json:"cdnOptimizationIssues,omitempty"`

	SurrogateMaxAge               int  `json:"surrogateMaxAge,omitempty"`
	SurrogateStaleWhileRevalidate int  `json:"surrogateStaleWhileRevalidate,omitempty"`
	SurrogateStaleIfError         int  `json:"surrogateStaleIfError,omitempty"`
	SurrogateNoStore              bool `json:"surrogateNoStore,omitempty"`
	SurrogateNoStoreRemote        bool `json:"surrogateNoStoreRemote,omitempty"`

	RequestID string `json:"requestId,omitempty"`

	CacheHitRateEstimate  string `json:"cacheHitRateEstimate,omitempty"`
	BandwidthSavingsLevel string `json:"bandwidthSavingsLevel,omitempty"`
	CacheCostAnalysis     string `json:"cacheCostAnalysis,omitempty"`

	DedupeApplied         bool     `json:"dedupeApplied,omitempty"`
	DuplicatesRemoved     int      `json:"duplicatesRemoved,omitempty"`
	DuplicateSignals      []string `json:"duplicateSignals,omitempty"`
	NearDuplicatesRemoved int      `json:"nearDuplicatesRemoved,omitempty"`
	NearDuplicateSignals  []string `json:"nearDuplicateSignals,omitempty"`
	OriginalBlockCount    int      `json:"originalBlockCount,omitempty"`
	UniqueBlockCount      int      `json:"uniqueBlockCount,omitempty"`

	Links    []LinkRef    `json:"links,omitempty"`
	Images   []ImageRef   `json:"images,omitempty"`
	Tables   []TableRef   `json:"tables,omitempty"`
	Comments []CommentRef `json:"comments,omitempty"`
	Chunks   []Chunk      `json:"chunks,omitempty"`

	SplitFiles []SplitFile `json:"splitFiles,omitempty"`

	RankedSections []RankedSection `json:"rankedSections,omitempty"`

	Schema map[string]interface{} `json:"schema,omitempty"`

	Warnings []string `json:"warnings,omitempty"`

	Truncated bool `json:"truncated,omitempty"`

	PruneFallbackUsed bool `json:"pruneFallbackUsed,omitempty"`

	ExtractionMethod string `json:"extractionMethod,omitempty"`
}

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
}
