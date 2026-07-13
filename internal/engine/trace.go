package engine

import "strings"

func computeTraceInfo(r *ResponseHeaders) TraceInfo {
	var formats []string
	var correlation string

	if r.ResponseTraceparent != "" {
		formats = append(formats, "w3c")
	}

	hasB3Multi := r.ResponseXB3TraceId != ""
	hasB3Single := r.ResponseB3 != ""
	if hasB3Multi || hasB3Single {
		formats = append(formats, "b3")
	}

	if r.ResponseXAmznTraceId != "" {
		formats = append(formats, "xray")
	}

	if r.ResponseXTraceId != "" {
		formats = append(formats, "generic")
	}

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

	return TraceInfo{TraceFormats: formats, TraceCorrelation: correlation}
}

func extractW3CTraceID(traceparent string) string {
	parts := strings.Split(traceparent, "-")
	if len(parts) >= 2 {
		return strings.ToLower(parts[1])
	}
	return ""
}

func extractB3SingleTraceID(b3 string) string {
	parts := strings.Split(b3, "-")
	if len(parts) >= 1 {
		return normalizeTraceID(parts[0])
	}
	return ""
}

func normalizeTraceID(traceID string) string {
	traceID = strings.ToLower(traceID)
	if len(traceID) == 16 {
		return "0000000000000000" + traceID
	}
	return traceID
}
