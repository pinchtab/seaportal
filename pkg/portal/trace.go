// Package portal provides content extraction with SPA detection
package portal

import "strings"

// computeTraceInfo analyzes tracing headers and returns formats present and correlation info.
// Returns (formats, correlation) where formats is a list of tracing standards present
// and correlation notes when B3 and W3C trace IDs match.
func computeTraceInfo(r Result) ([]string, string) {
	var formats []string
	var correlation string

	// Check W3C Trace Context (Traceparent header)
	if r.ResponseTraceparent != "" {
		formats = append(formats, "w3c")
	}

	// Check B3 format (either multi-header or single-header)
	hasB3Multi := r.ResponseXB3TraceId != ""
	hasB3Single := r.ResponseB3 != ""
	if hasB3Multi || hasB3Single {
		formats = append(formats, "b3")
	}

	// Check AWS X-Ray
	if r.ResponseXAmznTraceId != "" {
		formats = append(formats, "xray")
	}

	// Check generic trace ID
	if r.ResponseXTraceId != "" {
		formats = append(formats, "generic")
	}

	// Cross-format correlation: check if W3C and B3 trace IDs match
	// W3C Traceparent format: {version}-{trace-id}-{parent-id}-{flags}
	// B3 multi-header: X-B3-TraceId contains the trace-id directly
	// B3 single-header: {trace-id}-{span-id}[-{sampling}[-{parent-span-id}]]
	if r.ResponseTraceparent != "" && (hasB3Multi || hasB3Single) {
		w3cTraceID := extractW3CTraceID(r.ResponseTraceparent)
		var b3TraceID string
		if hasB3Multi {
			b3TraceID = normalizeTraceID(r.ResponseXB3TraceId)
		} else if hasB3Single {
			b3TraceID = extractB3SingleTraceID(r.ResponseB3)
		}
		if w3cTraceID != "" && b3TraceID != "" && w3cTraceID == b3TraceID {
			correlation = "b3-w3c-match"
		}
	}

	return formats, correlation
}

// extractW3CTraceID extracts the trace-id from a W3C Traceparent header.
// Format: {version}-{trace-id}-{parent-id}-{flags}
// Example: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
func extractW3CTraceID(traceparent string) string {
	parts := strings.Split(traceparent, "-")
	if len(parts) >= 2 {
		return strings.ToLower(parts[1])
	}
	return ""
}

// extractB3SingleTraceID extracts the trace-id from a B3 single-header.
// Format: {trace-id}-{span-id}[-{sampling}[-{parent-span-id}]]
// Example: 80f198ee56343ba864fe8b2a57d3eff7-e457b5a2e4d86bd1-1
func extractB3SingleTraceID(b3 string) string {
	parts := strings.Split(b3, "-")
	if len(parts) >= 1 {
		return normalizeTraceID(parts[0])
	}
	return ""
}

// normalizeTraceID normalizes a trace ID to lowercase 32-char hex for comparison.
// Handles both 64-bit (16 char) and 128-bit (32 char) trace IDs.
// 64-bit IDs are left-padded with zeros to 32 chars.
func normalizeTraceID(traceID string) string {
	traceID = strings.ToLower(traceID)
	if len(traceID) == 16 {
		return "0000000000000000" + traceID
	}
	return traceID
}
