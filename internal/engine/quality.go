package engine

import "github.com/pinchtab/seaportal/internal/quality"

type QualityMetrics = quality.Metrics

func ComputeQuality(markdown string) QualityMetrics {
	return quality.Compute(markdown)
}
