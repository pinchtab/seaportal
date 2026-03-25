// Package portal provides content extraction with SPA detection
package engine

import "strings"

// CDN provider constants
const (
	CDNCloudflare  = "cloudflare"
	CDNCloudFront  = "cloudfront"
	CDNFastly      = "fastly"
	CDNAkamai      = "akamai"
	CDNVarnish     = "varnish"
	CDNNetlify     = "netlify"
	CDNVercel      = "vercel"
	CDNAzure       = "azure"
	CDNGCP         = "gcp"
	CDNFly         = "fly"
	CDNDeno        = "deno"
	CDNHeroku      = "heroku"
	CDNRender      = "render"
	CDNRailway     = "railway"
	CDNShopify     = "shopify"
	CDNSquarespace = "squarespace"
	CDNWix         = "wix"
	CDNWebflow     = "webflow"
)

// ViaHop represents a single hop in the Via header chain.
// Format: [protocol] host [comment]
// Example: "1.1 varnish", "HTTP/1.1 cache.example.com (squid)", "1.1 google"
type ViaHop struct {
	Protocol string `json:"protocol,omitempty"` // HTTP protocol version (e.g., "1.1", "HTTP/1.1", "2")
	Host     string `json:"host,omitempty"`     // Proxy/CDN hostname or identifier
	Comment  string `json:"comment,omitempty"`  // Optional comment (e.g., "(squid)", "(Varnish)")
}

// fingerprintCDN detects the CDN provider based on response header combinations.
// Returns (provider, signals) where provider is a constant like CDNCloudflare
// and signals lists the headers/patterns that identified the provider.
func fingerprintCDN(r Result) (string, []string) {
	var signals []string

	// Priority 1: Cloudflare (very distinctive headers)
	if r.ResponseCFCacheStatus != "" {
		signals = append(signals, "CF-Cache-Status")
		return CDNCloudflare, signals
	}
	if r.ResponseXCloudflareWorkersRequestId != "" {
		signals = append(signals, "X-Cloudflare-Workers-Request-Id")
		return CDNCloudflare, signals
	}

	// Priority 2: CloudFront (Amazon)
	if r.ResponseXAmzCfId != "" {
		signals = append(signals, "X-Amz-Cf-Id")
		return CDNCloudFront, signals
	}
	// X-Cache with "cloudfront" in value
	if strings.Contains(strings.ToLower(r.ResponseXCache), "cloudfront") {
		signals = append(signals, "X-Cache:cloudfront")
		return CDNCloudFront, signals
	}

	// Priority 3: Fastly (X-Served-By + X-Fastly-Request-ID)
	if r.ResponseXFastlyRequestID != "" {
		signals = append(signals, "X-Fastly-Request-ID")
		return CDNFastly, signals
	}
	if r.ResponseXServedBy != "" && strings.Contains(strings.ToLower(r.ResponseXServedBy), "cache-") {
		signals = append(signals, "X-Served-By:cache-*")
		return CDNFastly, signals
	}

	// Priority 4: Akamai
	if r.ResponseXAkamaiRequestID != "" {
		signals = append(signals, "X-Akamai-Request-ID")
		return CDNAkamai, signals
	}
	if r.ResponseXAkamaiTransformed != "" {
		signals = append(signals, "X-Akamai-Transformed")
		return CDNAkamai, signals
	}
	if r.ResponseXAkamaiSessionInfo != "" {
		signals = append(signals, "X-Akamai-Session-Info")
		return CDNAkamai, signals
	}

	// Priority 5: Varnish
	if r.ResponseXVarnish != "" {
		signals = append(signals, "X-Varnish")
		return CDNVarnish, signals
	}

	// Priority 6: Platform-specific CDNs
	if r.ResponseXNetlifyRequestId != "" {
		signals = append(signals, "X-Netlify-Request-Id")
		return CDNNetlify, signals
	}
	if r.ResponseXVercelId != "" {
		signals = append(signals, "X-Vercel-Id")
		return CDNVercel, signals
	}
	if r.ResponseXAzureRef != "" {
		signals = append(signals, "X-Azure-Ref")
		return CDNAzure, signals
	}
	if r.ResponseXGCPRegion != "" {
		signals = append(signals, "X-GCP-Region")
		return CDNGCP, signals
	}
	if r.ResponseXFlyRequestId != "" {
		signals = append(signals, "X-Fly-Request-Id")
		return CDNFly, signals
	}
	if r.ResponseXDenoRegion != "" {
		signals = append(signals, "X-Deno-Region")
		return CDNDeno, signals
	}
	if r.ResponseXHerokuRequestId != "" {
		signals = append(signals, "X-Heroku-Request-Id")
		return CDNHeroku, signals
	}
	if r.ResponseXRenderRequestId != "" {
		signals = append(signals, "X-Render-Request-Id")
		return CDNRender, signals
	}
	if r.ResponseXRailwayRequestId != "" {
		signals = append(signals, "X-Railway-Request-Id")
		return CDNRailway, signals
	}
	if r.ResponseXShopifyRequestID != "" || r.ResponseXShopifyStage != "" {
		if r.ResponseXShopifyRequestID != "" {
			signals = append(signals, "X-Shopify-Request-ID")
		}
		if r.ResponseXShopifyStage != "" {
			signals = append(signals, "X-Shopify-Stage")
		}
		return CDNShopify, signals
	}
	if r.ResponseXSquarespaceRequestId != "" {
		signals = append(signals, "X-Squarespace-Request-Id")
		return CDNSquarespace, signals
	}
	if r.ResponseXWixRequestId != "" {
		signals = append(signals, "X-Wix-Request-Id")
		return CDNWix, signals
	}
	if r.ResponseXWebflowRequestId != "" {
		signals = append(signals, "X-Webflow-Request-Id")
		return CDNWebflow, signals
	}

	// Priority 7: Generic X-CDN header
	if r.ResponseXCDN != "" {
		signals = append(signals, "X-CDN:"+r.ResponseXCDN)
		return strings.ToLower(r.ResponseXCDN), signals
	}

	// Priority 8: Via header analysis for common CDNs
	if r.ResponseVia != "" {
		viaLower := strings.ToLower(r.ResponseVia)
		if strings.Contains(viaLower, "cloudfront") {
			signals = append(signals, "Via:cloudfront")
			return CDNCloudFront, signals
		}
		if strings.Contains(viaLower, "varnish") {
			signals = append(signals, "Via:varnish")
			return CDNVarnish, signals
		}
		if strings.Contains(viaLower, "akamai") {
			signals = append(signals, "Via:akamai")
			return CDNAkamai, signals
		}
	}

	return "", nil
}

// parseViaHeader parses the Via header into a slice of ViaHop structs.
// Via header format: [protocol] host [comment], [protocol] host [comment], ...
// Examples:
//   - "1.1 varnish"
//   - "HTTP/1.1 cache.example.com (squid)"
//   - "1.1 google, 1.1 varnish (Varnish/6.0)"
//   - "1.0 fred, 1.1 p.example.net"
func parseViaHeader(via string) []ViaHop {
	if via == "" {
		return nil
	}

	var hops []ViaHop
	// Split by comma (multiple hops)
	parts := strings.Split(via, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		hop := ViaHop{}

		// Check for comment at the end (in parentheses)
		if idx := strings.Index(part, "("); idx >= 0 {
			if endIdx := strings.LastIndex(part, ")"); endIdx > idx {
				hop.Comment = part[idx : endIdx+1]
				part = strings.TrimSpace(part[:idx])
			}
		}

		// Split remaining into protocol and host
		// Format: "1.1 host" or "HTTP/1.1 host"
		fields := strings.Fields(part)
		if len(fields) >= 2 {
			hop.Protocol = fields[0]
			hop.Host = fields[1]
		} else if len(fields) == 1 {
			// Just a host/identifier
			hop.Host = fields[0]
		}

		if hop.Host != "" || hop.Protocol != "" {
			hops = append(hops, hop)
		}
	}

	return hops
}

// extractCDNEdgeLocation extracts the edge location/region from CDN headers.
