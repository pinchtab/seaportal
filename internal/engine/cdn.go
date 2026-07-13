package engine

import "strings"

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

type ViaHop struct {
	Protocol string `json:"protocol,omitempty"`
	Host     string `json:"host,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

func fingerprintCDN(r *ResponseHeaders) CDNInfo {
	provider, signals := detectCDNProvider(r)
	return CDNInfo{CDNProvider: provider, CDNSignals: signals}
}

func detectCDNProvider(r *ResponseHeaders) (string, []string) {
	var signals []string

	if r.ResponseCFCacheStatus != "" {
		signals = append(signals, "CF-Cache-Status")
		return CDNCloudflare, signals
	}
	if r.ResponseXCloudflareWorkersRequestId != "" {
		signals = append(signals, "X-Cloudflare-Workers-Request-Id")
		return CDNCloudflare, signals
	}

	if r.ResponseXAmzCfId != "" {
		signals = append(signals, "X-Amz-Cf-Id")
		return CDNCloudFront, signals
	}
	if strings.Contains(strings.ToLower(r.ResponseXCache), "cloudfront") {
		signals = append(signals, "X-Cache:cloudfront")
		return CDNCloudFront, signals
	}

	if r.ResponseXFastlyRequestID != "" {
		signals = append(signals, "X-Fastly-Request-ID")
		return CDNFastly, signals
	}
	if r.ResponseXServedBy != "" && strings.Contains(strings.ToLower(r.ResponseXServedBy), "cache-") {
		signals = append(signals, "X-Served-By:cache-*")
		return CDNFastly, signals
	}

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

	if r.ResponseXVarnish != "" {
		signals = append(signals, "X-Varnish")
		return CDNVarnish, signals
	}

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

	if r.ResponseXCDN != "" {
		signals = append(signals, "X-CDN:"+r.ResponseXCDN)
		return strings.ToLower(r.ResponseXCDN), signals
	}

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

func parseViaHeader(via string) []ViaHop {
	if via == "" {
		return nil
	}

	var hops []ViaHop
	parts := strings.Split(via, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		hop := ViaHop{}

		if idx := strings.Index(part, "("); idx >= 0 {
			if endIdx := strings.LastIndex(part, ")"); endIdx > idx {
				hop.Comment = part[idx : endIdx+1]
				part = strings.TrimSpace(part[:idx])
			}
		}

		fields := strings.Fields(part)
		if len(fields) >= 2 {
			hop.Protocol = fields[0]
			hop.Host = fields[1]
		} else if len(fields) == 1 {
			hop.Host = fields[0]
		}

		if hop.Host != "" || hop.Protocol != "" {
			hops = append(hops, hop)
		}
	}

	return hops
}
