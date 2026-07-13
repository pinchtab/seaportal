package engine

import (
	"net/http"
)

func populateResponseHeaders(result *Result, resp *http.Response) {
	result.ResponseContentType = resp.Header.Get("Content-Type")
	result.populate(resp.Header)
}

func (h *ResponseHeaders) populate(hdr http.Header) {
	h.ResponseETag = hdr.Get("ETag")
	h.ResponseLastModified = hdr.Get("Last-Modified")

	h.ResponseContentEncoding = hdr.Get("Content-Encoding")
	h.ResponseServer = hdr.Get("Server")
	h.ResponseXForwardedFor = hdr.Get("X-Forwarded-For")

	h.ResponseVia = hdr.Get("Via")
	h.ResponseConnection = hdr.Get("Connection")
	h.ResponseAge = hdr.Get("Age")

	h.ResponseCacheControl = hdr.Get("Cache-Control")
	h.ResponseXCache = hdr.Get("X-Cache")
	h.ResponseVary = hdr.Get("Vary")

	h.ResponseXCacheHits = hdr.Get("X-Cache-Hits")
	h.ResponseSurrogateControl = hdr.Get("Surrogate-Control")
	h.ResponseCFCacheStatus = hdr.Get("CF-Cache-Status")

	h.ResponseXServedBy = hdr.Get("X-Served-By")
	h.ResponseXFastlyRequestID = hdr.Get("X-Fastly-Request-ID")

	h.ResponseXAkamaiTransformed = hdr.Get("X-Akamai-Transformed")
	h.ResponseXAkamaiSessionInfo = hdr.Get("X-Akamai-Session-Info")
	h.ResponseXAkamaiRequestID = hdr.Get("X-Akamai-Request-ID")

	h.ResponseXRequestId = hdr.Get("X-Request-Id")
	h.ResponseXCorrelationId = hdr.Get("X-Correlation-Id")

	h.ResponseXVarnish = hdr.Get("X-Varnish")

	h.ResponseXCDN = hdr.Get("X-CDN")

	h.ResponseXTraceId = hdr.Get("X-Trace-Id")
	h.ResponseXB3TraceId = hdr.Get("X-B3-TraceId")
	h.ResponseXB3SpanId = hdr.Get("X-B3-SpanId")
	h.ResponseXB3ParentSpanId = hdr.Get("X-B3-ParentSpanId")
	h.ResponseXB3Sampled = hdr.Get("X-B3-Sampled")
	h.ResponseB3 = hdr.Get("b3")
	h.ResponseTraceparent = hdr.Get("Traceparent")
	h.ResponseTracestate = hdr.Get("Tracestate")
	h.ResponseXAmznTraceId = hdr.Get("X-Amzn-Trace-Id")

	h.ResponseNEL = hdr.Get("NEL")

	h.ResponseReportTo = hdr.Get("Report-To")

	h.ResponsePermissionsPolicy = hdr.Get("Permissions-Policy")
	h.ResponseExpectCT = hdr.Get("Expect-CT")
	h.ResponseFeaturePolicy = hdr.Get("Feature-Policy")
	h.ResponseReportingEndpoints = hdr.Get("Reporting-Endpoints")
	h.ResponseCSP = hdr.Get("Content-Security-Policy")
	h.ResponseCSPReportOnly = hdr.Get("Content-Security-Policy-Report-Only")

	h.ResponseCORP = hdr.Get("Cross-Origin-Resource-Policy")
	h.ResponseCOEP = hdr.Get("Cross-Origin-Embedder-Policy")
	h.ResponseCOOP = hdr.Get("Cross-Origin-Opener-Policy")

	h.ResponseHSTS = hdr.Get("Strict-Transport-Security")

	h.ResponseXContentTypeOptions = hdr.Get("X-Content-Type-Options")
	h.ResponseXFrameOptions = hdr.Get("X-Frame-Options")

	h.ResponseReferrerPolicy = hdr.Get("Referrer-Policy")

	h.ResponseXXSSProtection = hdr.Get("X-XSS-Protection")
	h.ResponseXPermittedCrossDomainPolicies = hdr.Get("X-Permitted-Cross-Domain-Policies")
	h.ResponseXDownloadOptions = hdr.Get("X-Download-Options")

	h.ResponseClearSiteData = hdr.Get("Clear-Site-Data")

	h.ResponseTimingAllowOrigin = hdr.Get("Timing-Allow-Origin")

	h.ResponseOriginAgentCluster = hdr.Get("Origin-Agent-Cluster")

	h.ResponseDocumentPolicy = hdr.Get("Document-Policy")

	h.ResponseAcceptCH = hdr.Get("Accept-CH")

	h.ResponseSecCHUA = hdr.Get("Sec-CH-UA")
	h.ResponseSecCHUAMobile = hdr.Get("Sec-CH-UA-Mobile")
	h.ResponseSecCHUAPlatform = hdr.Get("Sec-CH-UA-Platform")
	h.ResponseSecCHUAFullVersionList = hdr.Get("Sec-CH-UA-Full-Version-List")
	h.ResponseSecCHPrefersColorScheme = hdr.Get("Sec-CH-Prefers-Color-Scheme")

	h.ResponseCriticalCH = hdr.Get("Critical-CH")

	h.ResponseCOEPReportOnly = hdr.Get("Cross-Origin-Embedder-Policy-Report-Only")
	h.ResponseCOOPReportOnly = hdr.Get("Cross-Origin-Opener-Policy-Report-Only")

	h.ResponseDocumentPolicyReportOnly = hdr.Get("Document-Policy-Report-Only")

	h.ResponseSourceMap = hdr.Get("SourceMap")
	if h.ResponseSourceMap == "" {
		h.ResponseSourceMap = hdr.Get("X-SourceMap")
	}

	h.ResponseAccessControlAllowOrigin = hdr.Get("Access-Control-Allow-Origin")
	h.ResponseAccessControlAllowMethods = hdr.Get("Access-Control-Allow-Methods")
	h.ResponseAccessControlAllowHeaders = hdr.Get("Access-Control-Allow-Headers")
	h.ResponseAccessControlAllowCredentials = hdr.Get("Access-Control-Allow-Credentials")
	h.ResponseAccessControlExposeHeaders = hdr.Get("Access-Control-Expose-Headers")
	h.ResponseAccessControlMaxAge = hdr.Get("Access-Control-Max-Age")

	h.ResponseLink = hdr.Get("Link")

	h.ResponseXRobotsTag = hdr.Get("X-Robots-Tag")

	h.ResponseContentDisposition = hdr.Get("Content-Disposition")

	h.ResponseXContentDuration = hdr.Get("X-Content-Duration")

	h.ResponseRefresh = hdr.Get("Refresh")

	h.ResponseContentLanguage = hdr.Get("Content-Language")

	h.ResponseXUACompatible = hdr.Get("X-UA-Compatible")

	h.ResponseAcceptRanges = hdr.Get("Accept-Ranges")

	h.ResponseTransferEncoding = hdr.Get("Transfer-Encoding")

	h.ResponseContentRange = hdr.Get("Content-Range")

	h.ResponsePragma = hdr.Get("Pragma")

	h.ResponseXPoweredBy = hdr.Get("X-Powered-By")

	h.ResponseXAspNetVersion = hdr.Get("X-AspNet-Version")
	h.ResponseXAspNetMvcVersion = hdr.Get("X-AspNetMvc-Version")
	h.ResponseServerTiming = hdr.Get("Server-Timing")

	h.ResponseXGenerator = hdr.Get("X-Generator")

	h.ResponseXRuntime = hdr.Get("X-Runtime")
	h.ResponseXDrupalCache = hdr.Get("X-Drupal-Cache")
	h.ResponseXMagentoCacheControl = hdr.Get("X-Magento-Cache-Control")
	h.ResponseXDrupalDynamicCache = hdr.Get("X-Drupal-Dynamic-Cache")
	h.ResponseXMagentoTags = hdr.Get("X-Magento-Tags")
	h.ResponseXShopifyStage = hdr.Get("X-Shopify-Stage")
	h.ResponseXShopifyRequestID = hdr.Get("X-Shopify-Request-ID")
	h.ResponseXWPTotal = hdr.Get("X-WP-Total")
	h.ResponseXWPTotalPages = hdr.Get("X-WP-TotalPages")
	h.ResponseXCraftCache = hdr.Get("X-Craft-Cache")
	h.ResponseXDiscourseRoute = hdr.Get("X-Discourse-Route")
	h.ResponseXGhostCacheStatus = hdr.Get("X-Ghost-Cache-Status")
	h.ResponseXJoomlaCache = hdr.Get("X-Joomla-Cache")
	h.ResponseXDiscourseMediaType = hdr.Get("X-Discourse-Media-Type")
	h.ResponseXPrestaShopCache = hdr.Get("X-PrestaShop-Cache")
	h.ResponseXMagentoCacheDebug = hdr.Get("X-Magento-Cache-Debug")
	h.ResponseXTypo3Cache = hdr.Get("X-Typo3-Cache")
	h.ResponseXWixRequestId = hdr.Get("X-Wix-Request-Id")
	h.ResponseXSquarespaceRequestId = hdr.Get("X-Squarespace-Request-Id")
	h.ResponseXWebflowRequestId = hdr.Get("X-Webflow-Request-Id")
	h.ResponseXContentfulRequestId = hdr.Get("X-Contentful-Request-Id")
	h.ResponseXNetlifyRequestId = hdr.Get("X-Netlify-Request-Id")
	h.ResponseXVercelId = hdr.Get("X-Vercel-Id")
	h.ResponseXHerokuRequestId = hdr.Get("X-Heroku-Request-Id")
	h.ResponseXRenderRequestId = hdr.Get("X-Render-Request-Id")
	h.ResponseXRailwayRequestId = hdr.Get("X-Railway-Request-Id")
	h.ResponseXFlyRequestId = hdr.Get("X-Fly-Request-Id")
	h.ResponseXDenoRegion = hdr.Get("X-Deno-Region")
	h.ResponseXCloudflareWorkersRequestId = hdr.Get("X-Cloudflare-Workers-Request-Id")
	h.ResponseXAzureRef = hdr.Get("X-Azure-Ref")
	h.ResponseXGCPRegion = hdr.Get("X-GCP-Region")
	h.ResponseXAmzCfId = hdr.Get("X-Amz-Cf-Id")
}
