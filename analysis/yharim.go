package analysis

import "math"

// YharimLimit computes the Yharim detectability threshold for a given
// false positive rate alpha. The formula is: Upsilon = sqrt(2 * log(1/alpha)).
//
// Special cases:
//   - alpha <= 0: returns +Inf (impossible false positive rate)
//   - alpha >= 1: returns 0 (everything is "detectable")
func YharimLimit(alpha float64) float64 {
	if alpha <= 0 {
		return math.Inf(1)
	}
	if alpha >= 1 {
		return 0
	}
	return math.Sqrt(2 * math.Log(1.0/alpha))
}

// SNR computes the signal-to-noise ratio for a set of tension values.
// SNR = (mean / stddev) * sqrt(n), where n is the cardinality of the set.
//
// Special cases:
//   - Empty slice: returns 0.
//   - Zero stddev with nonzero mean: returns +Inf.
//   - Zero mean: returns 0 (regardless of stddev).
func SNR(tensions []float64) float64 {
	n := len(tensions)
	if n == 0 {
		return 0
	}

	// Compute mean.
	sum := 0.0
	for _, v := range tensions {
		sum += v
	}
	mean := sum / float64(n)

	if mean == 0 {
		return 0
	}

	// Compute standard deviation (population stddev).
	variance := 0.0
	for _, v := range tensions {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(n)
	stddev := math.Sqrt(variance)

	if stddev == 0 {
		return math.Inf(1)
	}

	return (mean / stddev) * math.Sqrt(float64(n))
}

// IsDetectable returns true if the tension values indicate a detectable
// anomaly at the given false positive rate alpha.
// Returns true iff SNR(tensions) > YharimLimit(alpha).
func IsDetectable(tensions []float64, alpha float64) bool {
	return SNR(tensions) > YharimLimit(alpha)
}
