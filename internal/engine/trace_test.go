package engine

import (
	"reflect"
	"testing"
)

const (
	w3cTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	traceID128     = "4bf92f3577b34da6a3ce929d0e0e4736"
)

func TestComputeTraceInfo(t *testing.T) {
	tests := []struct {
		name            string
		headers         ResponseHeaders
		wantFormats     []string
		wantCorrelation string
	}{
		{
			name: "no tracing headers",
		},
		{
			name:        "w3c traceparent only",
			headers:     ResponseHeaders{ResponseTraceparent: w3cTraceparent},
			wantFormats: []string{"w3c"},
		},
		{
			name:        "b3 multi-header only",
			headers:     ResponseHeaders{ResponseXB3TraceId: traceID128},
			wantFormats: []string{"b3"},
		},
		{
			name:        "b3 single-header only",
			headers:     ResponseHeaders{ResponseB3: traceID128 + "-e457b5a2e4d86bd1-1"},
			wantFormats: []string{"b3"},
		},
		{
			name:        "b3 multi and single count once",
			headers:     ResponseHeaders{ResponseXB3TraceId: traceID128, ResponseB3: traceID128 + "-e457b5a2e4d86bd1"},
			wantFormats: []string{"b3"},
		},
		{
			name:        "aws xray",
			headers:     ResponseHeaders{ResponseXAmznTraceId: "Root=1-67891233-abcdef012345678912345678"},
			wantFormats: []string{"xray"},
		},
		{
			name:        "generic x-trace-id",
			headers:     ResponseHeaders{ResponseXTraceId: "abc123"},
			wantFormats: []string{"generic"},
		},
		{
			name: "all four formats",
			headers: ResponseHeaders{
				ResponseTraceparent:  w3cTraceparent,
				ResponseXB3TraceId:   "deadbeefdeadbeefdeadbeefdeadbeef",
				ResponseXAmznTraceId: "Root=1-abc",
				ResponseXTraceId:     "generic-id",
			},
			wantFormats: []string{"w3c", "b3", "xray", "generic"},
		},
		{
			name: "w3c and b3 multi with matching trace ids correlate",
			headers: ResponseHeaders{
				ResponseTraceparent: w3cTraceparent,
				ResponseXB3TraceId:  traceID128,
			},
			wantFormats:     []string{"w3c", "b3"},
			wantCorrelation: "b3-w3c-match",
		},
		{
			name: "matching is case-insensitive",
			headers: ResponseHeaders{
				ResponseTraceparent: w3cTraceparent,
				ResponseXB3TraceId:  "4BF92F3577B34DA6A3CE929D0E0E4736",
			},
			wantFormats:     []string{"w3c", "b3"},
			wantCorrelation: "b3-w3c-match",
		},
		{
			name: "w3c and b3 single with matching trace ids correlate",
			headers: ResponseHeaders{
				ResponseTraceparent: w3cTraceparent,
				ResponseB3:          traceID128 + "-e457b5a2e4d86bd1-1",
			},
			wantFormats:     []string{"w3c", "b3"},
			wantCorrelation: "b3-w3c-match",
		},
		{
			name: "64-bit b3 id is zero-padded before comparison",
			headers: ResponseHeaders{
				ResponseTraceparent: "00-0000000000000000a3ce929d0e0e4736-00f067aa0ba902b7-01",
				ResponseXB3TraceId:  "a3ce929d0e0e4736",
			},
			wantFormats:     []string{"w3c", "b3"},
			wantCorrelation: "b3-w3c-match",
		},
		{
			name: "different trace ids do not correlate",
			headers: ResponseHeaders{
				ResponseTraceparent: w3cTraceparent,
				ResponseXB3TraceId:  "deadbeefdeadbeefdeadbeefdeadbeef",
			},
			wantFormats: []string{"w3c", "b3"},
		},
		{
			name: "b3 multi wins over single when both present",
			headers: ResponseHeaders{
				ResponseTraceparent: w3cTraceparent,
				ResponseXB3TraceId:  "deadbeefdeadbeefdeadbeefdeadbeef",
				ResponseB3:          traceID128 + "-e457b5a2e4d86bd1",
			},
			wantFormats: []string{"w3c", "b3"},
		},
		{
			name: "malformed traceparent yields no correlation",
			headers: ResponseHeaders{
				ResponseTraceparent: "not-a-real-traceparent",
				ResponseXB3TraceId:  traceID128,
			},
			wantFormats: []string{"w3c", "b3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.headers
			got := computeTraceInfo(&h)
			if !reflect.DeepEqual(got.TraceFormats, tt.wantFormats) {
				t.Errorf("formats: got %v want %v", got.TraceFormats, tt.wantFormats)
			}
			if got.TraceCorrelation != tt.wantCorrelation {
				t.Errorf("correlation: got %q want %q", got.TraceCorrelation, tt.wantCorrelation)
			}
		})
	}
}

func TestExtractW3CTraceID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{w3cTraceparent, traceID128},
		{"00-ABCDEF-span-01", "abcdef"},
		{"justoneword", ""},
		{"00-", ""},
	}
	for _, tt := range tests {
		if got := extractW3CTraceID(tt.in); got != tt.want {
			t.Errorf("extractW3CTraceID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestExtractB3SingleTraceID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"80f198ee56343ba864fe8b2a57d3eff7-e457b5a2e4d86bd1-1", "80f198ee56343ba864fe8b2a57d3eff7"},
		{"a3ce929d0e0e4736-e457b5a2e4d86bd1", "0000000000000000a3ce929d0e0e4736"},
		{"traceonly", "traceonly"},
	}
	for _, tt := range tests {
		if got := extractB3SingleTraceID(tt.in); got != tt.want {
			t.Errorf("extractB3SingleTraceID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeTraceID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"A3CE929D0E0E4736", "0000000000000000a3ce929d0e0e4736"},
		{traceID128, traceID128},
		{"short", "short"},
	}
	for _, tt := range tests {
		if got := normalizeTraceID(tt.in); got != tt.want {
			t.Errorf("normalizeTraceID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
