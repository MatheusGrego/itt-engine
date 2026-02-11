package analysis

import (
	"fmt"
	"math"
)

// DetectabilityRegion classifies the detectability of anomalies in a set of tensions.
type DetectabilityRegion int

const (
	// Undetectable: SNR < Υ — no method can reliably detect anomalies.
	Undetectable DetectabilityRegion = iota
	// WeaklyDetectable: Υ < SNR < 2Υ — requires global analysis.
	WeaklyDetectable
	// StronglyDetectable: SNR > 2Υ — local analysis sufficient.
	StronglyDetectable
)

// String returns a human-readable name for the detectability region.
func (r DetectabilityRegion) String() string {
	switch r {
	case Undetectable:
		return "Undetectable"
	case WeaklyDetectable:
		return "WeaklyDetectable"
	case StronglyDetectable:
		return "StronglyDetectable"
	default:
		return fmt.Sprintf("DetectabilityRegion(%d)", int(r))
	}
}

// DetectabilityResult holds the full detectability analysis.
type DetectabilityResult struct {
	SNR       float64             // signal-to-noise ratio
	Threshold float64             // Υ(α) = Yharim limit
	Region    DetectabilityRegion // classified region
	Alpha     float64             // false positive rate used
}

// Classify returns the detectability region for a set of tensions
// at the given false positive rate alpha.
func Classify(tensions []float64, alpha float64) DetectabilityRegion {
	if len(tensions) == 0 {
		return Undetectable
	}
	snr := SNR(tensions)
	limit := YharimLimit(alpha)
	if snr > 2*limit {
		return StronglyDetectable
	}
	if snr > limit {
		return WeaklyDetectable
	}
	return Undetectable
}

// Detectability computes the full detectability analysis for a set of
// tension values at the given false positive rate alpha.
func Detectability(tensions []float64, alpha float64) DetectabilityResult {
	snr := 0.0
	if len(tensions) > 0 {
		snr = SNR(tensions)
	}
	return DetectabilityResult{
		SNR:       snr,
		Threshold: YharimLimit(alpha),
		Region:    Classify(tensions, alpha),
		Alpha:     alpha,
	}
}

// CPS computes the Concealment Probability Score for a region.
// Returns 0 if the region is below the detectability threshold (SNR <= Yharim limit).
// Otherwise returns a normalized score in [0, 1] based on concealment cost
// relative to mean tension.
//
// Formula: CPS(Sigma) = normalize(Omega) * 1_{SNR > Y(alpha)}
func CPS(tensions []float64, concealmentCost float64, alpha float64) float64 {
	if !IsDetectable(tensions, alpha) {
		return 0
	}
	if len(tensions) == 0 || concealmentCost <= 0 {
		return 0
	}
	// Compute mean tension
	sum := 0.0
	for _, t := range tensions {
		sum += t
	}
	meanT := sum / float64(len(tensions))
	if meanT <= 0 {
		return 0
	}
	// Sigmoid normalization: maps (0, inf) -> (0, 1)
	ratio := concealmentCost / meanT
	return 2.0/(1.0+math.Exp(-ratio)) - 1.0
}
