package main

import (
	"math"
	"slices"
)

type number interface {
	~int64 | ~float64
}

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
