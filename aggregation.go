package itt

import "sort"

// Built-in aggregation functions for use with Builder.AggregationFunc().
var (
	// AggMean computes the arithmetic mean of tension values.
	AggMean AggregationFunc = func(tensions []float64) float64 {
		if len(tensions) == 0 {
			return 0
		}
		sum := 0.0
		for _, t := range tensions {
			sum += t
		}
		return sum / float64(len(tensions))
	}

	// AggMax returns the maximum tension value.
	AggMax AggregationFunc = func(tensions []float64) float64 {
		if len(tensions) == 0 {
			return 0
		}
		max := tensions[0]
		for _, t := range tensions[1:] {
			if t > max {
				max = t
			}
		}
		return max
	}

	// AggMedian computes the median tension value.
	AggMedian AggregationFunc = func(tensions []float64) float64 {
		if len(tensions) == 0 {
			return 0
		}
		sorted := make([]float64, len(tensions))
		copy(sorted, tensions)
		sort.Float64s(sorted)
		n := len(sorted)
		if n%2 == 0 {
			return (sorted[n/2-1] + sorted[n/2]) / 2.0
		}
		return sorted[n/2]
	}

	// AggSum returns the sum of all tension values.
	AggSum AggregationFunc = func(tensions []float64) float64 {
		sum := 0.0
		for _, t := range tensions {
			sum += t
		}
		return sum
	}
)
