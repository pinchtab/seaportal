package main

// Shared numeric helpers for the bench lanes. Percentile and mean were
// previously reimplemented per file with three different nearest-rank
// variants; they are consolidated here on the canonical nearest-rank
// formula (0-indexed rank ceil(q*n)-1).

import (
	"math"
	"slices"
)

// number is the value set the bench lanes aggregate: nanosecond /
// millisecond counts (int64, time.Duration) and ratios (float64).
type number interface {
	~int64 | ~float64
}

// percentile returns the q-quantile (q in (0,1]) of xs using the canonical
// nearest-rank method: the value at 1-indexed rank ceil(q*n), clamped to a
// valid index. Sorts a copy so the caller's slice is untouched. Empty input
// returns the zero value.
func percentile[T number](xs []T, q float64) T {
	if len(xs) == 0 {
		var zero T
		return zero
	}
	cp := make([]T, len(xs))
	copy(cp, xs)
	slices.Sort(cp)
	idx := int(math.Ceil(q*float64(len(cp)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

// mean returns the arithmetic mean of xs, or 0 for empty input.
func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}
