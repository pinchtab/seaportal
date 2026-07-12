package engine

import "testing"

func TestSameHost_DefaultPortNormalization(t *testing.T) {
	tests := []struct {
		name       string
		rawURL     string
		baseHost   string
		baseScheme string
		want       bool
	}{
		{"identical no port", "http://example.com/a", "example.com", "http", true},
		{"base has default http port", "http://example.com/a", "example.com:80", "http", true},
		{"member has default http port", "http://example.com:80/a", "example.com", "http", true},
		{"both default http port", "http://example.com:80/a", "example.com:80", "http", true},
		{"https default port both ways", "https://example.com/a", "example.com:443", "https", true},
		{"member omits https default", "https://example.com/a", "example.com:443", "https", true},
		{"different host", "http://other.com/a", "example.com:80", "http", false},
		{"non-default port differs", "http://example.com:8080/a", "example.com", "http", false},
		{"non-default port matches", "http://example.com:8080/a", "example.com:8080", "http", true},
		{"case-insensitive host", "http://Example.COM/a", "example.com", "http", true},
		{"scheme-relative member inherits base scheme", "//example.com:80/a", "example.com", "http", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameHost(tt.rawURL, tt.baseHost, tt.baseScheme); got != tt.want {
				t.Errorf("sameHost(%q, %q, %q) = %v, want %v", tt.rawURL, tt.baseHost, tt.baseScheme, got, tt.want)
			}
		})
	}
}
