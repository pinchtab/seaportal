// Package portal provides content extraction with SPA detection
package engine

import "time"

// Result holds extraction output with confidence scoring
type Result struct {
	URL        string   `json:"url"`
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	Byline     string   `json:"byline"`
	Excerpt    string   `json:"excerpt"`
	SiteName   string   `json:"sitename"`
	Length     int      `json:"length"`
	TimeMs     int64    `json:"timeMs"`
	Confidence int      `json:"confidence"`
	IsSPA      bool     `json:"isSpa"`
	IsBlocked  bool     `json:"isBlocked"`
	SPASignals []string `json:"spaSignals,omitempty"`
	Error      string   `json:"error,omitempty"`
	FromCache  bool     `json:"fromCache,omitempty"`
	StatusCode int      `json:"statusCode,omitempty"` // HTTP status code (0 if from cache/fixture)

	HeadingCount   int `json:"headingCount"`
	LinkCount      int `json:"linkCount"`
	ParagraphCount int `json:"paragraphCount"`

	Quality     float64        `json:"quality"` // 0-100 extraction quality score
	QualityInfo QualityMetrics `json:"qualityInfo"`
	Profile     PageProfile    `json:"profile"`
	Fingerprint string         `json:"fingerprint"` // Semantic content fingerprint (stable across noise)
	Validation  Validation     `json:"validation"`

	// Retry metrics for observability
	RetryCount          int           `json:"retryCount,omitempty"`
	TotalRetryWait      time.Duration `json:"totalRetryWait,omitempty"`      // Cumulative wait time across retries
	HeadPreflightStatus int           `json:"headPreflightStatus,omitempty"` // HTTP status from HEAD preflight (0 if not used)

	// Extraction timing breakdown for performance analysis
	FetchTimeMs   int64 `json:"fetchTimeMs,omitempty"`
	ParseTimeMs   int64 `json:"parseTimeMs,omitempty"` // Time spent parsing HTML (readability)
	ConvertTimeMs int64 `json:"convertTimeMs,omitempty"`

	// Response metadata for bandwidth and content analysis
	ContentLength       int64  `json:"contentLength,omitempty"` // Response body size in bytes
	ResponseContentType string `json:"responseContentType,omitempty"`

	// Redirect tracking for observability
	RedirectCount int      `json:"redirectCount,omitempty"`
	RedirectChain []string `json:"redirectChain,omitempty"` // URLs in redirect chain (excluding final URL)
	FinalURL      string   `json:"finalUrl,omitempty"`      // Final URL after all redirects (for canonicalization)

	// Soft-404 detection
	IsSoft404    bool     `json:"isSoft404,omitempty"` // True if page looks like an error page despite 200 status
	Soft404Hints []string `json:"soft404Hints,omitempty"`

	// Response caching headers for conditional request support
	ResponseETag         string `json:"responseEtag,omitempty"`         // ETag header from response
	ResponseLastModified string `json:"responseLastModified,omitempty"` // Last-Modified header from response

	// Response metadata for debugging/analytics
	ResponseContentEncoding string `json:"responseContentEncoding,omitempty"` // Content-Encoding header (gzip, br, etc.)
	ResponseServer          string `json:"responseServer,omitempty"`          // Server header for debugging/analytics
	ResponseXForwardedFor   string `json:"responseXForwardedFor,omitempty"`   // X-Forwarded-For header from response (for proxy chain analysis)

	// Request/response encoding tracking
	RequestAcceptEncoding string `json:"requestAcceptEncoding,omitempty"` // Accept-Encoding we sent in request

	// Response timing breakdown (granular)
	TTFBMs     int64 `json:"ttfbMs,omitempty"`     // Time to first byte (request start to headers received)
	DownloadMs int64 `json:"downloadMs,omitempty"` // Time spent downloading body

	// HTTP proxy and caching headers
	ResponseVia        string `json:"responseVia,omitempty"`        // Via header for HTTP proxy chain analysis
	ResponseConnection string `json:"responseConnection,omitempty"` // Connection header for keep-alive behavior
	ResponseAge        string `json:"responseAge,omitempty"`        // Age header for CDN cache freshness analysis

	// Cache policy and CDN headers
	ResponseCacheControl string `json:"responseCacheControl,omitempty"` // Cache-Control header for cache policy analysis
	ResponseXCache       string `json:"responseXCache,omitempty"`       // X-Cache header for CDN cache hit detection
	ResponseVary         string `json:"responseVary,omitempty"`         // Vary header for content negotiation analysis

	// CDN-specific headers
	ResponseXCacheHits       string `json:"responseXCacheHits,omitempty"`       // X-Cache-Hits header for CDN hit count analysis
	ResponseSurrogateControl string `json:"responseSurrogateControl,omitempty"` // Surrogate-Control header for CDN-specific cache policies
	ResponseCFCacheStatus    string `json:"responseCfCacheStatus,omitempty"`    // CF-Cache-Status header for Cloudflare cache status

	// Fastly-specific headers
	ResponseXServedBy        string `json:"responseXServedBy,omitempty"`        // X-Served-By header for Fastly server identification
	ResponseXFastlyRequestID string `json:"responseXFastlyRequestId,omitempty"` // X-Fastly-Request-ID header for Fastly request tracing

	// Akamai-specific headers
	ResponseXAkamaiTransformed string `json:"responseXAkamaiTransformed,omitempty"` // X-Akamai-Transformed header for Akamai content transformation status
	ResponseXAkamaiSessionInfo string `json:"responseXAkamaiSessionInfo,omitempty"` // X-Akamai-Session-Info header for Akamai session information
	ResponseXAkamaiRequestID   string `json:"responseXAkamaiRequestId,omitempty"`   // X-Akamai-Request-ID header for Akamai request tracing

	// Request correlation headers (generic)
	ResponseXRequestId     string `json:"responseXRequestId,omitempty"`     // X-Request-Id header for request correlation
	ResponseXCorrelationId string `json:"responseXCorrelationId,omitempty"` // X-Correlation-Id header for distributed tracing

	// Varnish-specific headers
	ResponseXVarnish string `json:"responseXVarnish,omitempty"` // X-Varnish header for Varnish cache server identification

	// Generic CDN headers
	ResponseXCDN string `json:"responseXCdn,omitempty"` // X-CDN header for CDN provider identification

	// OpenTelemetry / Zipkin distributed tracing headers
	ResponseXTraceId        string `json:"responseXTraceId,omitempty"`        // X-Trace-Id header for OpenTelemetry/generic tracing compatibility
	ResponseXB3TraceId      string `json:"responseXB3TraceId,omitempty"`      // X-B3-TraceId header for Zipkin distributed tracing
	ResponseXB3SpanId       string `json:"responseXB3SpanId,omitempty"`       // X-B3-SpanId header for Zipkin span identification
	ResponseXB3ParentSpanId string `json:"responseXB3ParentSpanId,omitempty"` // X-B3-ParentSpanId header for Zipkin parent span identification
	ResponseXB3Sampled      string `json:"responseXB3Sampled,omitempty"`      // X-B3-Sampled header for Zipkin trace sampling decisions (0 or 1)
	ResponseB3              string `json:"responseB3,omitempty"`              // B3 single-header format (trace-id-span-id-sampling-parent-span-id)
	ResponseTraceparent     string `json:"responseTraceparent,omitempty"`     // Traceparent header for W3C Trace Context compatibility
	ResponseTracestate      string `json:"responseTracestate,omitempty"`      // Tracestate header for W3C Trace Context vendor-specific data
	ResponseXAmznTraceId    string `json:"responseXAmznTraceId,omitempty"`    // X-Amzn-Trace-Id header for AWS X-Ray tracing

	// Trace correlation and completeness
	TraceFormats     []string `json:"traceFormats,omitempty"`     // Which tracing formats are present: w3c, b3, xray, generic
	TraceCorrelation string   `json:"traceCorrelation,omitempty"` // Notes when B3 and W3C trace IDs correlate (same trace-id)

	// Network Error Logging (browser error reporting)
	ResponseNEL string `json:"responseNel,omitempty"` // NEL header for Network Error Logging configuration

	// Report-To header (endpoint configuration for NEL and other reporting APIs)
	ResponseReportTo string `json:"responseReportTo,omitempty"` // Report-To header for browser reporting endpoint configuration

	// Browser security and feature policy headers
	ResponsePermissionsPolicy  string `json:"responsePermissionsPolicy,omitempty"`  // Permissions-Policy header for browser feature access control
	ResponseExpectCT           string `json:"responseExpectCt,omitempty"`           // Expect-CT header for Certificate Transparency enforcement
	ResponseFeaturePolicy      string `json:"responseFeaturePolicy,omitempty"`      // Feature-Policy header (legacy, predecessor to Permissions-Policy)
	ResponseReportingEndpoints string `json:"responseReportingEndpoints,omitempty"` // Reporting-Endpoints header (newer alternative to Report-To)
	ResponseCSP                string `json:"responseCsp,omitempty"`                // Content-Security-Policy header for security policy enforcement
	ResponseCSPReportOnly      string `json:"responseCspReportOnly,omitempty"`      // Content-Security-Policy-Report-Only header for testing/monitoring policies

	// Cross-Origin isolation headers
	ResponseCORP string `json:"responseCorp,omitempty"` // Cross-Origin-Resource-Policy header for resource sharing control
	ResponseCOEP string `json:"responseCoep,omitempty"` // Cross-Origin-Embedder-Policy header for embedding control
	ResponseCOOP string `json:"responseCoop,omitempty"` // Cross-Origin-Opener-Policy header for opener control

	// Transport security
	ResponseHSTS string `json:"responseHsts,omitempty"` // Strict-Transport-Security header for TLS policy enforcement

	// Legacy security headers
	ResponseXContentTypeOptions string `json:"responseXContentTypeOptions,omitempty"` // X-Content-Type-Options header for MIME type sniffing control (typically "nosniff")
	ResponseXFrameOptions       string `json:"responseXFrameOptions,omitempty"`       // X-Frame-Options header for legacy clickjacking protection (DENY, SAMEORIGIN, ALLOW-FROM)

	// Privacy/referrer control
	ResponseReferrerPolicy string `json:"responseReferrerPolicy,omitempty"` // Referrer-Policy header for controlling referrer information sent with requests

	// Legacy security headers (deprecated but still common)
	ResponseXXSSProtection                string `json:"responseXXssProtection,omitempty"`                // X-XSS-Protection header for legacy XSS filter control (deprecated, "1; mode=block" was common)
	ResponseXPermittedCrossDomainPolicies string `json:"responseXPermittedCrossDomainPolicies,omitempty"` // X-Permitted-Cross-Domain-Policies header for Flash/PDF cross-domain control
	ResponseXDownloadOptions              string `json:"responseXDownloadOptions,omitempty"`              // X-Download-Options header for IE download behavior control ("noopen")

	// Privacy and session control
	ResponseClearSiteData string `json:"responseClearSiteData,omitempty"` // Clear-Site-Data header for clearing browser data (cookies, storage, cache) on logout

	// Performance API access control
	ResponseTimingAllowOrigin string `json:"responseTimingAllowOrigin,omitempty"` // Timing-Allow-Origin header for cross-origin Resource Timing API access

	// Process isolation
	ResponseOriginAgentCluster string `json:"responseOriginAgentCluster,omitempty"` // Origin-Agent-Cluster header for process isolation hints (?1 enables)

	// Document feature control
	ResponseDocumentPolicy string `json:"responseDocumentPolicy,omitempty"` // Document-Policy header for document feature control (force-load-at-top, etc.)

	// Client hints negotiation
	ResponseAcceptCH string `json:"responseAcceptCH,omitempty"` // Accept-CH header for client hints negotiation (device-memory, viewport-width, etc.)

	// Client hints response headers (sent by server to indicate what hints it received/supports)
	ResponseSecCHUA                 string `json:"responseSecChUa,omitempty"`                 // Sec-CH-UA header for user-agent brand/version hints
	ResponseSecCHUAMobile           string `json:"responseSecChUaMobile,omitempty"`           // Sec-CH-UA-Mobile header for mobile device detection (?0 or ?1)
	ResponseSecCHUAPlatform         string `json:"responseSecChUaPlatform,omitempty"`         // Sec-CH-UA-Platform header for operating system identification
	ResponseSecCHUAFullVersionList  string `json:"responseSecChUaFullVersionList,omitempty"`  // Sec-CH-UA-Full-Version-List header for detailed browser versions
	ResponseSecCHPrefersColorScheme string `json:"responseSecChPrefersColorScheme,omitempty"` // Sec-CH-Prefers-Color-Scheme header for dark/light mode preference

	// Critical client hints (require page reload if not provided)
	ResponseCriticalCH string `json:"responseCriticalCh,omitempty"` // Critical-CH header for critical client hints that require page reload

	// Cross-origin policy report-only headers (for testing without enforcement)
	ResponseCOEPReportOnly string `json:"responseCoepReportOnly,omitempty"` // Cross-Origin-Embedder-Policy-Report-Only header for COEP testing
	ResponseCOOPReportOnly string `json:"responseCoopReportOnly,omitempty"` // Cross-Origin-Opener-Policy-Report-Only header for COOP testing

	// Document policy report-only header (for testing without enforcement)
	ResponseDocumentPolicyReportOnly string `json:"responseDocumentPolicyReportOnly,omitempty"` // Document-Policy-Report-Only header for testing document policies

	// Source map header for debugging/development detection
	ResponseSourceMap string `json:"responseSourceMap,omitempty"` // SourceMap header for source map file location (indicates development/debug builds)

	// CORS response headers (server responses to cross-origin requests)
	ResponseAccessControlAllowOrigin      string `json:"responseAccessControlAllowOrigin,omitempty"`      // Access-Control-Allow-Origin header for CORS allowed origins
	ResponseAccessControlAllowMethods     string `json:"responseAccessControlAllowMethods,omitempty"`     // Access-Control-Allow-Methods header for allowed HTTP methods
	ResponseAccessControlAllowHeaders     string `json:"responseAccessControlAllowHeaders,omitempty"`     // Access-Control-Allow-Headers header for allowed request headers
	ResponseAccessControlAllowCredentials string `json:"responseAccessControlAllowCredentials,omitempty"` // Access-Control-Allow-Credentials header for credentials support
	ResponseAccessControlExposeHeaders    string `json:"responseAccessControlExposeHeaders,omitempty"`    // Access-Control-Expose-Headers header for exposed response headers
	ResponseAccessControlMaxAge           string `json:"responseAccessControlMaxAge,omitempty"`           // Access-Control-Max-Age header for preflight cache duration

	// Link header for resource hints and preloading
	ResponseLink string `json:"responseLink,omitempty"` // Link header for preload, prefetch, preconnect hints

	// Robots and indexing control
	ResponseXRobotsTag string `json:"responseXRobotsTag,omitempty"` // X-Robots-Tag header for page indexing directives (noindex, nofollow, etc.)

	// Content disposition for download/attachment detection
	ResponseContentDisposition string `json:"responseContentDisposition,omitempty"` // Content-Disposition header for inline vs attachment handling

	// Media duration hint
	ResponseXContentDuration string `json:"responseXContentDuration,omitempty"` // X-Content-Duration header for media file duration hints (seconds)

	// HTTP-level refresh/redirect
	ResponseRefresh string `json:"responseRefresh,omitempty"` // Refresh header for HTTP-level redirect/meta refresh (e.g., "5; url=https://example.com")

	// Content language detection
	ResponseContentLanguage string `json:"responseContentLanguage,omitempty"` // Content-Language header for response language detection (e.g., "en-US", "de, en")

	// IE compatibility mode
	ResponseXUACompatible string `json:"responseXUaCompatible,omitempty"` // X-UA-Compatible header for IE compatibility mode detection (e.g., "IE=edge", "IE=11")

	// Range/resume support headers
	ResponseAcceptRanges string `json:"responseAcceptRanges,omitempty"` // Accept-Ranges header for byte-range/resume support detection (typically "bytes" or "none")

	// Transfer encoding
	ResponseTransferEncoding string `json:"responseTransferEncoding,omitempty"` // Transfer-Encoding header for chunked response detection (e.g., "chunked", "gzip, chunked")

	// Partial content tracking
	ResponseContentRange string `json:"responseContentRange,omitempty"` // Content-Range header for partial content tracking (206 responses, e.g., "bytes 0-1023/10240")

	// Legacy cache control
	ResponsePragma string `json:"responsePragma,omitempty"` // Pragma header for legacy HTTP/1.0 cache control (typically "no-cache")

	// Server technology fingerprinting
	ResponseXPoweredBy string `json:"responseXPoweredBy,omitempty"` // X-Powered-By header for server technology detection (e.g., "PHP/8.2.0", "Express", "ASP.NET")

	// .NET version detection
	ResponseXAspNetVersion string `json:"responseXAspNetVersion,omitempty"` // X-AspNet-Version header for .NET framework version detection (e.g., "4.0.30319")

	// MVC framework version detection
	ResponseXAspNetMvcVersion string `json:"responseXAspNetMvcVersion,omitempty"` // X-AspNetMvc-Version header for ASP.NET MVC framework version (e.g., "5.2", "4.0")

	// Server performance timing
	ResponseServerTiming string `json:"responseServerTiming,omitempty"` // Server-Timing header for performance metrics (e.g., "db;dur=53", "cache;desc=Cache Read;dur=23.2")

	// CMS/generator fingerprinting
	ResponseXGenerator string `json:"responseXGenerator,omitempty"` // X-Generator header for CMS/generator identification (e.g., "Drupal 10", "WordPress 6.0")

	// Ruby/Rails timing
	ResponseXRuntime string `json:"responseXRuntime,omitempty"` // X-Runtime header for Ruby/Rails request timing (e.g., "0.123456", "0.042")

	// Drupal-specific cache status
	ResponseXDrupalCache string `json:"responseXDrupalCache,omitempty"` // X-Drupal-Cache header for Drupal cache status (e.g., "HIT", "MISS", "UNCACHEABLE")

	// Magento-specific cache control
	ResponseXMagentoCacheControl string `json:"responseXMagentoCacheControl,omitempty"` // X-Magento-Cache-Control header for Magento cache configuration (e.g., "max-age=86400", "no-cache")

	// Drupal 8+ dynamic page caching
	ResponseXDrupalDynamicCache string `json:"responseXDrupalDynamicCache,omitempty"` // X-Drupal-Dynamic-Cache header for Drupal 8+ dynamic page caching (e.g., "HIT", "MISS", "UNCACHEABLE")

	// Magento cache tag invalidation
	ResponseXMagentoTags string `json:"responseXMagentoTags,omitempty"` // X-Magento-Tags header for Magento cache tag invalidation tracking (e.g., "cms_p_1,store")

	// Shopify deployment stage
	ResponseXShopifyStage string `json:"responseXShopifyStage,omitempty"` // X-Shopify-Stage header for Shopify deployment stage detection (e.g., "production", "staging")

	// Shopify request tracing
	ResponseXShopifyRequestID string `json:"responseXShopifyRequestId,omitempty"` // X-Shopify-Request-ID header for Shopify request tracing (unique request identifier)

	// WordPress REST API pagination headers
	ResponseXWPTotal      string `json:"responseXWpTotal,omitempty"`      // X-WP-Total header for WordPress REST API total item count (e.g., "42", "1000")
	ResponseXWPTotalPages string `json:"responseXWpTotalPages,omitempty"` // X-WP-TotalPages header for WordPress REST API pagination (e.g., "5", "10")

	// Craft CMS cache status
	ResponseXCraftCache string `json:"responseXCraftCache,omitempty"` // X-Craft-Cache header for Craft CMS cache status (e.g., "hit", "miss", "bypass")

	// Discourse forum routing
	ResponseXDiscourseRoute string `json:"responseXDiscourseRoute,omitempty"` // X-Discourse-Route header for Discourse forum routing information (e.g., "topics/show", "categories/index")

	// Ghost blog cache status
	ResponseXGhostCacheStatus string `json:"responseXGhostCacheStatus,omitempty"` // X-Ghost-Cache-Status header for Ghost blog cache detection (e.g., "HIT", "MISS", "BYPASS")

	// Joomla CMS cache status
	ResponseXJoomlaCache string `json:"responseXJoomlaCache,omitempty"` // X-Joomla-Cache header for Joomla CMS cache status (e.g., "HIT", "MISS", "EXPIRED")

	// Discourse content type detection
	ResponseXDiscourseMediaType string `json:"responseXDiscourseMediaType,omitempty"` // X-Discourse-Media-Type header for Discourse content type detection (e.g., "text/html", "application/json")

	// PrestaShop e-commerce cache status
	ResponseXPrestaShopCache string `json:"responseXPrestaShopCache,omitempty"` // X-PrestaShop-Cache header for PrestaShop e-commerce cache status (e.g., "HIT", "MISS", "BYPASS")

	// Magento debug mode detection
	ResponseXMagentoCacheDebug string `json:"responseXMagentoCacheDebug,omitempty"` // X-Magento-Cache-Debug header for Magento debug mode detection (e.g., "HIT", "MISS", "1", "0")

	// TYPO3 CMS cache status
	ResponseXTypo3Cache string `json:"responseXTypo3Cache,omitempty"` // X-Typo3-Cache header for TYPO3 CMS cache status (e.g., "HIT", "MISS", "BYPASS")

	// Wix platform request tracing
	ResponseXWixRequestId string `json:"responseXWixRequestId,omitempty"` // X-Wix-Request-Id header for Wix platform request tracing (unique request identifier)

	// Squarespace platform request tracing
	ResponseXSquarespaceRequestId string `json:"responseXSquarespaceRequestId,omitempty"` // X-Squarespace-Request-Id header for Squarespace request tracing (unique request identifier)

	// Webflow platform request tracing
	ResponseXWebflowRequestId string `json:"responseXWebflowRequestId,omitempty"` // X-Webflow-Request-Id header for Webflow platform request tracing (unique request identifier)

	// Contentful CMS request tracing
	ResponseXContentfulRequestId string `json:"responseXContentfulRequestId,omitempty"` // X-Contentful-Request-Id header for Contentful CMS request tracing (unique request identifier)

	// Netlify platform request tracing
	ResponseXNetlifyRequestId string `json:"responseXNetlifyRequestId,omitempty"` // X-Netlify-Request-Id header for Netlify deployment request tracing (unique request identifier)

	// Vercel platform request identification
	ResponseXVercelId string `json:"responseXVercelId,omitempty"` // X-Vercel-Id header for Vercel platform request identification (includes region and request ID)

	// Heroku platform request tracing
	ResponseXHerokuRequestId string `json:"responseXHerokuRequestId,omitempty"` // X-Heroku-Request-Id header for Heroku platform request tracing (UUID format)

	// Render platform request tracing
	ResponseXRenderRequestId string `json:"responseXRenderRequestId,omitempty"` // X-Render-Request-Id header for Render platform request tracing (unique request identifier)

	// Railway platform request tracing
	ResponseXRailwayRequestId string `json:"responseXRailwayRequestId,omitempty"` // X-Railway-Request-Id header for Railway platform request tracing (unique request identifier)

	// Fly.io platform request tracing
	ResponseXFlyRequestId string `json:"responseXFlyRequestId,omitempty"` // X-Fly-Request-Id header for Fly.io platform request tracing (unique request identifier)

	// Deno Deploy region identification
	ResponseXDenoRegion string `json:"responseXDenoRegion,omitempty"` // X-Deno-Region header for Deno Deploy region identification (edge region where request was processed)

	// Cloudflare Workers request tracing
	ResponseXCloudflareWorkersRequestId string `json:"responseXCloudflareWorkersRequestId,omitempty"` // X-Cloudflare-Workers-Request-Id header for Cloudflare Workers request tracing (unique request identifier)

	// Azure CDN/Front Door request tracing
	ResponseXAzureRef string `json:"responseXAzureRef,omitempty"` // X-Azure-Ref header for Azure CDN/Front Door request tracing (reference ID for Azure support and debugging)

	// Google Cloud CDN/Load Balancer region identification
	ResponseXGCPRegion string `json:"responseXGcpRegion,omitempty"` // X-GCP-Region header for Google Cloud CDN/Load Balancer region identification (edge region where request was processed)

	// CloudFront request identification
	ResponseXAmzCfId string `json:"responseXAmzCfId,omitempty"` // X-Amz-Cf-Id header for CloudFront request identification (unique request ID for CloudFront edge debugging)

	// Normalized cache status (aggregates X-Cache, CF-Cache-Status, X-Drupal-Cache, etc.)
	NormalizedCacheStatus string `json:"normalizedCacheStatus,omitempty"` // Normalized cache status: HIT, MISS, BYPASS, EXPIRED, STALE, DYNAMIC, REVALIDATED, or empty if unknown
	CacheStatusSource     string `json:"cacheStatusSource,omitempty"`     // Header that provided the cache status (e.g., "CF-Cache-Status", "X-Cache", "X-Drupal-Cache")

	// CDN fingerprinting (identifies CDN provider based on header combinations)
	CDNProvider string   `json:"cdnProvider,omitempty"` // Detected CDN provider: cloudflare, cloudfront, fastly, akamai, varnish, netlify, vercel, azure, gcp, fly, deno, heroku, render, railway, unknown
	CDNSignals  []string `json:"cdnSignals,omitempty"`  // Headers/patterns that identified the CDN provider

	// Via header analysis for multi-tier CDN/proxy detection
	ViaHops     []ViaHop `json:"viaHops,omitempty"`     // Parsed Via header chain (protocol, hostname, optional comment per hop)
	ProxyLayers int      `json:"proxyLayers,omitempty"` // Number of proxy/CDN layers detected in Via chain

	// Cache efficiency metrics
	CacheAge       int  `json:"cacheAge,omitempty"`       // Age header value in seconds (how long content has been cached)
	CacheMaxAge    int  `json:"cacheMaxAge,omitempty"`    // max-age directive from Cache-Control in seconds
	CacheSMaxAge   int  `json:"cacheSMaxAge,omitempty"`   // s-maxage directive from Cache-Control in seconds (shared/CDN cache TTL)
	CacheFreshness int  `json:"cacheFreshness,omitempty"` // Remaining freshness: max-age - age (negative means stale)
	CacheIsStale   bool `json:"cacheIsStale,omitempty"`   // True if cached content is past max-age

	// Cache revalidation directives
	CacheStaleWhileRevalidate int  `json:"cacheStaleWhileRevalidate,omitempty"` // stale-while-revalidate directive in seconds (async revalidation window)
	CacheStaleIfError         int  `json:"cacheStaleIfError,omitempty"`         // stale-if-error directive in seconds (serve stale on origin error)
	CacheHasSWR               bool `json:"cacheHasSwr,omitempty"`               // True if stale-while-revalidate directive is present

	// CDN edge location
	CDNEdgeLocation string `json:"cdnEdgeLocation,omitempty"` // Extracted edge/POP location from CDN request IDs (e.g., "iad1", "lhr", "fra")

	// Cache revalidation policy directives
	CacheMustRevalidate  bool     `json:"cacheMustRevalidate,omitempty"`  // True if must-revalidate directive is present (stale content must be revalidated)
	CacheProxyRevalidate bool     `json:"cacheProxyRevalidate,omitempty"` // True if proxy-revalidate directive is present (CDN-specific must-revalidate)
	CacheNoCache         bool     `json:"cacheNoCache,omitempty"`         // True if no-cache directive is present (must revalidate before serving)
	CacheNoCacheFields   []string `json:"cacheNoCacheFields,omitempty"`   // Field names from no-cache="field1, field2" (specific headers that cannot be served from cache)
	CacheNoStore         bool     `json:"cacheNoStore,omitempty"`         // True if no-store directive is present (content should not be cached at all)
	CacheImmutable       bool     `json:"cacheImmutable,omitempty"`       // True if immutable directive is present (content will never change, can cache forever)
	CachePrivate         bool     `json:"cachePrivate,omitempty"`         // True if private directive is present (browser-only caching, no shared/CDN cache)
	CachePublic          bool     `json:"cachePublic,omitempty"`          // True if public directive is present (shared/CDN cache allowed)

	// Cache policy summarization and quality metrics
	CachePolicySummary string `json:"cachePolicySummary,omitempty"` // Cache behavior: aggressive, moderate, revalidate, no-cache, no-store, or empty if no caching headers
	CachePolicyScore   int    `json:"cachePolicyScore,omitempty"`   // Cache efficiency score 0-100 (higher = better caching for performance)

	// Effective CDN TTL and optimization analysis
	EffectiveCDNTTL       int      `json:"effectiveCdnTtl,omitempty"`       // Effective TTL for CDN caches in seconds (considers both Cache-Control and Surrogate-Control)
	CacheRecommendations  []string `json:"cacheRecommendations,omitempty"`  // Actionable suggestions for improving cache policy
	CDNOptimizationIssues []string `json:"cdnOptimizationIssues,omitempty"` // Detected missing CDN optimizations

	// Surrogate-Control header parsing (CDN-specific cache overrides)
	SurrogateMaxAge               int  `json:"surrogateMaxAge,omitempty"`               // max-age from Surrogate-Control (CDN-specific TTL override)
	SurrogateStaleWhileRevalidate int  `json:"surrogateStaleWhileRevalidate,omitempty"` // stale-while-revalidate from Surrogate-Control
	SurrogateStaleIfError         int  `json:"surrogateStaleIfError,omitempty"`         // stale-if-error from Surrogate-Control
	SurrogateNoStore              bool `json:"surrogateNoStore,omitempty"`              // no-store from Surrogate-Control
	SurrogateNoStoreRemote        bool `json:"surrogateNoStoreRemote,omitempty"`        // no-store-remote from Surrogate-Control (edge caching only)

	// Distributed tracing
	RequestID string `json:"requestId,omitempty"` // Optional request ID for distributed tracing/logging

	// Cache cost estimation
	CacheHitRateEstimate  string `json:"cacheHitRateEstimate,omitempty"`  // Estimated cache hit rate: "excellent" (95%+), "good" (80%+), "moderate" (50%+), "low" (<50%), "none" (0%)
	BandwidthSavingsLevel string `json:"bandwidthSavingsLevel,omitempty"` // Estimated bandwidth savings: "maximum", "high", "moderate", "low", "none"
	CacheCostAnalysis     string `json:"cacheCostAnalysis,omitempty"`     // Human-readable cache cost analysis summary

	// Content deduplication metrics
	DedupeApplied      bool     `json:"dedupeApplied,omitempty"`      // Whether deduplication was applied
	DuplicatesRemoved  int      `json:"duplicatesRemoved,omitempty"`  // Number of duplicate blocks removed
	DuplicateSignals   []string `json:"duplicateSignals,omitempty"`   // Types of duplicates found (nav, footer, heading, etc.)
	OriginalBlockCount int      `json:"originalBlockCount,omitempty"` // Number of blocks before deduplication
	UniqueBlockCount   int      `json:"uniqueBlockCount,omitempty"`   // Number of blocks after deduplication
}

// RetryEvent holds information about a retry attempt for debug logging.
type RetryEvent struct {
	Attempt    int           // Current attempt number (1-indexed)
	StatusCode int           // HTTP status code that triggered retry (0 for connection errors)
	WaitTime   time.Duration // Time to wait before next attempt
	Error      error         // Connection error if applicable
	Outcome    string        // "retrying", "success", "exhausted", "timeout"
}

// DomainRetry holds per-domain retry policy overrides.
type DomainRetry struct {
	MaxRetries   int           // Override MaxRetries for this domain
	MaxRetryWait time.Duration // Override MaxRetryWait for this domain
}

// Options configures extraction behavior
type Options struct {
	FailFast             bool
	FastMode             bool // Quick bail on SPA/bot detection without full extraction
	NoPooling            bool
	MaxRetries           int                      // Maximum number of retries for 429 responses (0 = no retry)
	MaxRetryWait         time.Duration            // Maximum wait time for Retry-After header (default 60s)
	TotalRetryTimeout    time.Duration            // Maximum cumulative retry wait time (default 120s); 0 = no limit
	HeadPreflight        bool                     // Optional HEAD request to detect permanent errors before GET
	ContentTypePreflight bool                     // HEAD request to skip binary content (images, PDFs, etc.)
	RetryLogger          func(event RetryEvent)   // Optional callback for retry debug logging
	DomainRetryConfig    map[string]DomainRetry   // Per-domain retry overrides (keyed by domain)
	DomainUserAgent      map[string]string        // Per-domain User-Agent overrides (keyed by domain)
	DomainTimeout        map[string]time.Duration // Per-domain HTTP timeout overrides (keyed by domain)
	RespectCrawlDelay    bool                     // When true, check robots.txt for Crawl-delay before fetching
	CrawlDelayCache      *CrawlDelayCache         // Optional shared crawl-delay cache (created automatically if nil and RespectCrawlDelay is true)
	RequestID            string                   // Optional request ID for distributed tracing/logging
	SendRequestID        bool                     // When true, send RequestID as X-Request-ID header to target server
	Dedupe               bool                     // When true, remove duplicate blocks from extracted content
}
