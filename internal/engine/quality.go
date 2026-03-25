package engine

import "github.com/pinchtab/seaportal/internal/quality"

// QualityMetrics is an alias for quality.Metrics to preserve API compatibility
type QualityMetrics = quality.Metrics

func ComputeQuality(markdown string) QualityMetrics {
	return quality.Compute(markdown)
}
