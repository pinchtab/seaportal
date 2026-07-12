package engine

import (
	"reflect"
	"testing"
)

// TestFingerprintCDN_Providers exercises the provider fingerprint table:
// each distinctive header maps to its provider constant and reports the
// header(s) that identified it as signals.
func TestFingerprintCDN_Providers(t *testing.T) {
	tests := []struct {
		name         string
		headers      ResponseHeaders
		wantProvider string
		wantSignals  []string
	}{
		{
			name:         "cloudflare cache status",
			headers:      ResponseHeaders{ResponseCFCacheStatus: "HIT"},
			wantProvider: CDNCloudflare,
			wantSignals:  []string{"CF-Cache-Status"},
		},
		{
			name:         "cloudflare workers request id",
			headers:      ResponseHeaders{ResponseXCloudflareWorkersRequestId: "abc"},
			wantProvider: CDNCloudflare,
			wantSignals:  []string{"X-Cloudflare-Workers-Request-Id"},
		},
		{
			name:         "cloudfront amz id",
			headers:      ResponseHeaders{ResponseXAmzCfId: "xyz=="},
			wantProvider: CDNCloudFront,
			wantSignals:  []string{"X-Amz-Cf-Id"},
		},
		{
			name:         "cloudfront via x-cache value case-insensitive",
			headers:      ResponseHeaders{ResponseXCache: "Hit from CloudFront"},
			wantProvider: CDNCloudFront,
			wantSignals:  []string{"X-Cache:cloudfront"},
		},
		{
			name:         "fastly request id",
			headers:      ResponseHeaders{ResponseXFastlyRequestID: "f1"},
			wantProvider: CDNFastly,
			wantSignals:  []string{"X-Fastly-Request-ID"},
		},
		{
			name:         "fastly served-by cache pop",
			headers:      ResponseHeaders{ResponseXServedBy: "cache-lhr7365-LHR"},
			wantProvider: CDNFastly,
			wantSignals:  []string{"X-Served-By:cache-*"},
		},
		{
			name:    "served-by without cache- prefix is not fastly",
			headers: ResponseHeaders{ResponseXServedBy: "origin-server-3"},
		},
		{
			name:         "akamai request id",
			headers:      ResponseHeaders{ResponseXAkamaiRequestID: "a1"},
			wantProvider: CDNAkamai,
			wantSignals:  []string{"X-Akamai-Request-ID"},
		},
		{
			name:         "akamai transformed",
			headers:      ResponseHeaders{ResponseXAkamaiTransformed: "9 - 0 pmb=mRUM,1"},
			wantProvider: CDNAkamai,
			wantSignals:  []string{"X-Akamai-Transformed"},
		},
		{
			name:         "varnish",
			headers:      ResponseHeaders{ResponseXVarnish: "123456 654321"},
			wantProvider: CDNVarnish,
			wantSignals:  []string{"X-Varnish"},
		},
		{
			name:         "netlify",
			headers:      ResponseHeaders{ResponseXNetlifyRequestId: "n1"},
			wantProvider: CDNNetlify,
			wantSignals:  []string{"X-Netlify-Request-Id"},
		},
		{
			name:         "vercel",
			headers:      ResponseHeaders{ResponseXVercelId: "v1"},
			wantProvider: CDNVercel,
			wantSignals:  []string{"X-Vercel-Id"},
		},
		{
			name:         "azure",
			headers:      ResponseHeaders{ResponseXAzureRef: "ref"},
			wantProvider: CDNAzure,
			wantSignals:  []string{"X-Azure-Ref"},
		},
		{
			name:         "gcp",
			headers:      ResponseHeaders{ResponseXGCPRegion: "europe-west1"},
			wantProvider: CDNGCP,
			wantSignals:  []string{"X-GCP-Region"},
		},
		{
			name:         "fly",
			headers:      ResponseHeaders{ResponseXFlyRequestId: "fly1"},
			wantProvider: CDNFly,
			wantSignals:  []string{"X-Fly-Request-Id"},
		},
		{
			name:         "shopify both signals",
			headers:      ResponseHeaders{ResponseXShopifyRequestID: "s1", ResponseXShopifyStage: "production"},
			wantProvider: CDNShopify,
			wantSignals:  []string{"X-Shopify-Request-ID", "X-Shopify-Stage"},
		},
		{
			name:         "generic X-CDN echoes the lowercased value",
			headers:      ResponseHeaders{ResponseXCDN: "Imperva"},
			wantProvider: "imperva",
			wantSignals:  []string{"X-CDN:Imperva"},
		},
		{
			name:         "via fallback cloudfront",
			headers:      ResponseHeaders{ResponseVia: "1.1 abc.cloudfront.net (CloudFront)"},
			wantProvider: CDNCloudFront,
			wantSignals:  []string{"Via:cloudfront"},
		},
		{
			name:         "via fallback varnish",
			headers:      ResponseHeaders{ResponseVia: "1.1 varnish (Varnish/6.0)"},
			wantProvider: CDNVarnish,
			wantSignals:  []string{"Via:varnish"},
		},
		{
			name:         "via fallback akamai",
			headers:      ResponseHeaders{ResponseVia: "1.1 akamai.net(ghost)"},
			wantProvider: CDNAkamai,
			wantSignals:  []string{"Via:akamai"},
		},
		{
			name: "no signals at all",
		},
		{
			name:    "unrelated via header yields no provider",
			headers: ResponseHeaders{ResponseVia: "1.1 proxy.corp.example (squid)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.headers
			got := fingerprintCDN(&h)
			if got.CDNProvider != tt.wantProvider {
				t.Errorf("provider: got %q want %q", got.CDNProvider, tt.wantProvider)
			}
			if !reflect.DeepEqual(got.CDNSignals, tt.wantSignals) {
				t.Errorf("signals: got %v want %v", got.CDNSignals, tt.wantSignals)
			}
		})
	}
}

// TestFingerprintCDN_Priority documents the fingerprint precedence: Cloudflare
// outranks CloudFront, which outranks Fastly, and any distinctive header
// outranks the generic X-CDN and Via fallbacks.
func TestFingerprintCDN_Priority(t *testing.T) {
	h := ResponseHeaders{
		ResponseCFCacheStatus:    "HIT",
		ResponseXAmzCfId:         "amz",
		ResponseXFastlyRequestID: "f1",
		ResponseXCDN:             "other",
		ResponseVia:              "1.1 varnish",
	}
	got := fingerprintCDN(&h)
	if got.CDNProvider != CDNCloudflare {
		t.Errorf("provider: got %q want %q (Cloudflare wins)", got.CDNProvider, CDNCloudflare)
	}

	h2 := ResponseHeaders{
		ResponseXAmzCfId:         "amz",
		ResponseXFastlyRequestID: "f1",
	}
	got2 := fingerprintCDN(&h2)
	if got2.CDNProvider != CDNCloudFront {
		t.Errorf("provider: got %q want %q (CloudFront beats Fastly)", got2.CDNProvider, CDNCloudFront)
	}
}

func TestParseViaHeader(t *testing.T) {
	tests := []struct {
		name string
		via  string
		want []ViaHop
	}{
		{
			name: "empty header",
			via:  "",
			want: nil,
		},
		{
			name: "single hop protocol and host",
			via:  "1.1 varnish",
			want: []ViaHop{{Protocol: "1.1", Host: "varnish"}},
		},
		{
			name: "full protocol form with comment",
			via:  "HTTP/1.1 cache.example.com (squid)",
			want: []ViaHop{{Protocol: "HTTP/1.1", Host: "cache.example.com", Comment: "(squid)"}},
		},
		{
			name: "multiple hops with comment",
			via:  "1.1 google, 1.1 varnish (Varnish/6.0)",
			want: []ViaHop{
				{Protocol: "1.1", Host: "google"},
				{Protocol: "1.1", Host: "varnish", Comment: "(Varnish/6.0)"},
			},
		},
		{
			name: "rfc example",
			via:  "1.0 fred, 1.1 p.example.net",
			want: []ViaHop{
				{Protocol: "1.0", Host: "fred"},
				{Protocol: "1.1", Host: "p.example.net"},
			},
		},
		{
			name: "bare identifier becomes host",
			via:  "varnish",
			want: []ViaHop{{Host: "varnish"}},
		},
		{
			name: "blank segments are skipped",
			via:  "1.1 a, , 1.1 b",
			want: []ViaHop{
				{Protocol: "1.1", Host: "a"},
				{Protocol: "1.1", Host: "b"},
			},
		},
		{
			name: "unclosed comment parenthesis is left in place",
			via:  "1.1 host (unclosed",
			want: []ViaHop{{Protocol: "1.1", Host: "host"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseViaHeader(tt.via)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseViaHeader(%q) = %#v, want %#v", tt.via, got, tt.want)
			}
		})
	}
}
