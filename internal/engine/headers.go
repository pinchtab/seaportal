// Package portal provides content extraction with SPA detection
package engine

import (
	"net/http"
)

// populateResponseHeaders copies all HTTP response headers to the Result struct.
// This is a mechanical mapping of headers to Result fields for observability.
func populateResponseHeaders(result *Result, resp *http.Response) {
	result.ResponseContentType = resp.Header.Get("Content-Type")

	// Response caching headers for conditional request support
	result.ResponseETag = resp.Header.Get("ETag")
	result.ResponseLastModified = resp.Header.Get("Last-Modified")

	// Response metadata for debugging/analytics
	result.ResponseContentEncoding = resp.Header.Get("Content-Encoding")
	result.ResponseServer = resp.Header.Get("Server")
	result.ResponseXForwardedFor = resp.Header.Get("X-Forwarded-For")

	// HTTP proxy and caching headers
	result.ResponseVia = resp.Header.Get("Via")
	result.ResponseConnection = resp.Header.Get("Connection")
	result.ResponseAge = resp.Header.Get("Age")

	// Cache policy and CDN headers
	result.ResponseCacheControl = resp.Header.Get("Cache-Control")
	result.ResponseXCache = resp.Header.Get("X-Cache")
	result.ResponseVary = resp.Header.Get("Vary")

	// CDN-specific headers
	result.ResponseXCacheHits = resp.Header.Get("X-Cache-Hits")
	result.ResponseSurrogateControl = resp.Header.Get("Surrogate-Control")
	result.ResponseCFCacheStatus = resp.Header.Get("CF-Cache-Status")

	// Fastly-specific headers
	result.ResponseXServedBy = resp.Header.Get("X-Served-By")
	result.ResponseXFastlyRequestID = resp.Header.Get("X-Fastly-Request-ID")

	// Akamai-specific headers
	result.ResponseXAkamaiTransformed = resp.Header.Get("X-Akamai-Transformed")
	result.ResponseXAkamaiSessionInfo = resp.Header.Get("X-Akamai-Session-Info")
	result.ResponseXAkamaiRequestID = resp.Header.Get("X-Akamai-Request-ID")

	// Request correlation headers (generic)
	result.ResponseXRequestId = resp.Header.Get("X-Request-Id")
	result.ResponseXCorrelationId = resp.Header.Get("X-Correlation-Id")

	// Varnish-specific headers
	result.ResponseXVarnish = resp.Header.Get("X-Varnish")

	// Generic CDN headers
	result.ResponseXCDN = resp.Header.Get("X-CDN")

	// OpenTelemetry / Zipkin distributed tracing headers
	result.ResponseXTraceId = resp.Header.Get("X-Trace-Id")
	result.ResponseXB3TraceId = resp.Header.Get("X-B3-TraceId")
	result.ResponseXB3SpanId = resp.Header.Get("X-B3-SpanId")
	result.ResponseXB3ParentSpanId = resp.Header.Get("X-B3-ParentSpanId")
	result.ResponseXB3Sampled = resp.Header.Get("X-B3-Sampled")
	result.ResponseB3 = resp.Header.Get("b3")
	result.ResponseTraceparent = resp.Header.Get("Traceparent")
	result.ResponseTracestate = resp.Header.Get("Tracestate")
	result.ResponseXAmznTraceId = resp.Header.Get("X-Amzn-Trace-Id")

	result.ResponseNEL = resp.Header.Get("NEL")

	// Report-To header (endpoint configuration for NEL and other reporting APIs)
	result.ResponseReportTo = resp.Header.Get("Report-To")

	// Browser security and feature policy headers
	result.ResponsePermissionsPolicy = resp.Header.Get("Permissions-Policy")
	result.ResponseExpectCT = resp.Header.Get("Expect-CT")
	result.ResponseFeaturePolicy = resp.Header.Get("Feature-Policy")
	result.ResponseReportingEndpoints = resp.Header.Get("Reporting-Endpoints")
	result.ResponseCSP = resp.Header.Get("Content-Security-Policy")
	result.ResponseCSPReportOnly = resp.Header.Get("Content-Security-Policy-Report-Only")

	// Cross-Origin isolation headers
	result.ResponseCORP = resp.Header.Get("Cross-Origin-Resource-Policy")
	result.ResponseCOEP = resp.Header.Get("Cross-Origin-Embedder-Policy")
	result.ResponseCOOP = resp.Header.Get("Cross-Origin-Opener-Policy")

	// Transport security
	result.ResponseHSTS = resp.Header.Get("Strict-Transport-Security")

	// Legacy security headers
	result.ResponseXContentTypeOptions = resp.Header.Get("X-Content-Type-Options")
	result.ResponseXFrameOptions = resp.Header.Get("X-Frame-Options")

	// Privacy/referrer control
	result.ResponseReferrerPolicy = resp.Header.Get("Referrer-Policy")

	// Legacy security headers (deprecated but still common)
	result.ResponseXXSSProtection = resp.Header.Get("X-XSS-Protection")
	result.ResponseXPermittedCrossDomainPolicies = resp.Header.Get("X-Permitted-Cross-Domain-Policies")
	result.ResponseXDownloadOptions = resp.Header.Get("X-Download-Options")

	// Privacy and session control
	result.ResponseClearSiteData = resp.Header.Get("Clear-Site-Data")

	// Performance API access control
	result.ResponseTimingAllowOrigin = resp.Header.Get("Timing-Allow-Origin")

	// Process isolation
	result.ResponseOriginAgentCluster = resp.Header.Get("Origin-Agent-Cluster")

	// Document feature control
	result.ResponseDocumentPolicy = resp.Header.Get("Document-Policy")

	// Client hints negotiation
	result.ResponseAcceptCH = resp.Header.Get("Accept-CH")

	// Client hints response headers
	result.ResponseSecCHUA = resp.Header.Get("Sec-CH-UA")
	result.ResponseSecCHUAMobile = resp.Header.Get("Sec-CH-UA-Mobile")
	result.ResponseSecCHUAPlatform = resp.Header.Get("Sec-CH-UA-Platform")
	result.ResponseSecCHUAFullVersionList = resp.Header.Get("Sec-CH-UA-Full-Version-List")
	result.ResponseSecCHPrefersColorScheme = resp.Header.Get("Sec-CH-Prefers-Color-Scheme")

	// Critical client hints (require page reload if not provided)
	result.ResponseCriticalCH = resp.Header.Get("Critical-CH")

	// Cross-origin policy report-only headers
	result.ResponseCOEPReportOnly = resp.Header.Get("Cross-Origin-Embedder-Policy-Report-Only")
	result.ResponseCOOPReportOnly = resp.Header.Get("Cross-Origin-Opener-Policy-Report-Only")

	// Document policy report-only header
	result.ResponseDocumentPolicyReportOnly = resp.Header.Get("Document-Policy-Report-Only")

	// Source map header (also check legacy X-SourceMap)
	result.ResponseSourceMap = resp.Header.Get("SourceMap")
	if result.ResponseSourceMap == "" {
		result.ResponseSourceMap = resp.Header.Get("X-SourceMap")
	}

	// CORS response headers
	result.ResponseAccessControlAllowOrigin = resp.Header.Get("Access-Control-Allow-Origin")
	result.ResponseAccessControlAllowMethods = resp.Header.Get("Access-Control-Allow-Methods")
	result.ResponseAccessControlAllowHeaders = resp.Header.Get("Access-Control-Allow-Headers")
	result.ResponseAccessControlAllowCredentials = resp.Header.Get("Access-Control-Allow-Credentials")
	result.ResponseAccessControlExposeHeaders = resp.Header.Get("Access-Control-Expose-Headers")
	result.ResponseAccessControlMaxAge = resp.Header.Get("Access-Control-Max-Age")

	result.ResponseLink = resp.Header.Get("Link")

	// Robots and indexing control
	result.ResponseXRobotsTag = resp.Header.Get("X-Robots-Tag")

	// Content disposition for download/attachment detection
	result.ResponseContentDisposition = resp.Header.Get("Content-Disposition")

	result.ResponseXContentDuration = resp.Header.Get("X-Content-Duration")

	// HTTP-level refresh/redirect
	result.ResponseRefresh = resp.Header.Get("Refresh")

	result.ResponseContentLanguage = resp.Header.Get("Content-Language")

	// IE compatibility mode
	result.ResponseXUACompatible = resp.Header.Get("X-UA-Compatible")

	result.ResponseAcceptRanges = resp.Header.Get("Accept-Ranges")

	// Transfer encoding (chunked response detection)
	result.ResponseTransferEncoding = resp.Header.Get("Transfer-Encoding")

	// Partial content tracking (206 responses)
	result.ResponseContentRange = resp.Header.Get("Content-Range")

	// Legacy cache control (HTTP/1.0 compatibility)
	result.ResponsePragma = resp.Header.Get("Pragma")

	// Server technology fingerprinting
	result.ResponseXPoweredBy = resp.Header.Get("X-Powered-By")

	result.ResponseXAspNetVersion = resp.Header.Get("X-AspNet-Version")
	result.ResponseXAspNetMvcVersion = resp.Header.Get("X-AspNetMvc-Version")
	result.ResponseServerTiming = resp.Header.Get("Server-Timing")

	// CMS/generator fingerprinting
	result.ResponseXGenerator = resp.Header.Get("X-Generator")

	result.ResponseXRuntime = resp.Header.Get("X-Runtime")
	result.ResponseXDrupalCache = resp.Header.Get("X-Drupal-Cache")
	result.ResponseXMagentoCacheControl = resp.Header.Get("X-Magento-Cache-Control")
	result.ResponseXDrupalDynamicCache = resp.Header.Get("X-Drupal-Dynamic-Cache")
	result.ResponseXMagentoTags = resp.Header.Get("X-Magento-Tags")
	result.ResponseXShopifyStage = resp.Header.Get("X-Shopify-Stage")
	result.ResponseXShopifyRequestID = resp.Header.Get("X-Shopify-Request-ID")
	result.ResponseXWPTotal = resp.Header.Get("X-WP-Total")
	result.ResponseXWPTotalPages = resp.Header.Get("X-WP-TotalPages")
	result.ResponseXCraftCache = resp.Header.Get("X-Craft-Cache")
	result.ResponseXDiscourseRoute = resp.Header.Get("X-Discourse-Route")
	result.ResponseXGhostCacheStatus = resp.Header.Get("X-Ghost-Cache-Status")
	result.ResponseXJoomlaCache = resp.Header.Get("X-Joomla-Cache")
	result.ResponseXDiscourseMediaType = resp.Header.Get("X-Discourse-Media-Type")
	result.ResponseXPrestaShopCache = resp.Header.Get("X-PrestaShop-Cache")
	result.ResponseXMagentoCacheDebug = resp.Header.Get("X-Magento-Cache-Debug")
	result.ResponseXTypo3Cache = resp.Header.Get("X-Typo3-Cache")
	result.ResponseXWixRequestId = resp.Header.Get("X-Wix-Request-Id")
	result.ResponseXSquarespaceRequestId = resp.Header.Get("X-Squarespace-Request-Id")
	result.ResponseXWebflowRequestId = resp.Header.Get("X-Webflow-Request-Id")
	result.ResponseXContentfulRequestId = resp.Header.Get("X-Contentful-Request-Id")
	result.ResponseXNetlifyRequestId = resp.Header.Get("X-Netlify-Request-Id")
	result.ResponseXVercelId = resp.Header.Get("X-Vercel-Id")
	result.ResponseXHerokuRequestId = resp.Header.Get("X-Heroku-Request-Id")
	result.ResponseXRenderRequestId = resp.Header.Get("X-Render-Request-Id")
	result.ResponseXRailwayRequestId = resp.Header.Get("X-Railway-Request-Id")
	result.ResponseXFlyRequestId = resp.Header.Get("X-Fly-Request-Id")
	result.ResponseXDenoRegion = resp.Header.Get("X-Deno-Region")
	result.ResponseXCloudflareWorkersRequestId = resp.Header.Get("X-Cloudflare-Workers-Request-Id")
	result.ResponseXAzureRef = resp.Header.Get("X-Azure-Ref")
	result.ResponseXGCPRegion = resp.Header.Get("X-GCP-Region")
	result.ResponseXAmzCfId = resp.Header.Get("X-Amz-Cf-Id")
}
