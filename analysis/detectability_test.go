package analysis

import (
	"math"
	"testing"
)

func TestClassify_StronglyDetectable(t *testing.T) {
	// All values identical and large => infinite SNR (stddev=0, mean>0).
	// Infinite SNR is always > 2*Υ, so StronglyDetectable.
	tensions := make([]float64, 100)
	for i := range tensions {
		tensions[i] = 5.0
	}
	region := Classify(tensions, 0.05)
	if region != StronglyDetectable {
		t.Fatalf("expected StronglyDetectable, got %v", region)
	}
}

func TestClassify_WeaklyDetectable(t *testing.T) {
	// We need Υ < SNR < 2Υ.
	// For alpha=0.05: Υ = sqrt(2*log(1/0.05)) = sqrt(2*2.9957) ≈ 2.448.
	// SNR = (mean/stddev)*sqrt(n). We need SNR in (2.448, 4.896).
	// Use n=1: SNR = mean/stddev. We need mean/stddev ≈ 3.5.
	// E.g., values with mean ≈ 3.5 and stddev ≈ 1.0.
	// A sample: {2.5, 3.5, 4.5} -> mean=3.5, stddev=0.8165, n=3, SNR = (3.5/0.8165)*sqrt(3) ≈ 7.42 (too high).
	//
	// Better approach: n=1 single sample. SNR = mean/stddev. But stddev of single sample
	// is 0 => Inf. We need n > 1.
	//
	// For n=4: SNR = (mean/stddev)*2. Need SNR in (2.448, 4.896) => mean/stddev in (1.224, 2.448).
	// Try values {1.0, 2.0, 3.0, 4.0}: mean=2.5, var=1.25, std=1.118, SNR=(2.5/1.118)*2=4.47.
	// 4.47 is in (2.448, 4.896). Good.
	tensions := []float64{1.0, 2.0, 3.0, 4.0}
	region := Classify(tensions, 0.05)
	if region != WeaklyDetectable {
		snr := SNR(tensions)
		limit := YharimLimit(0.05)
		t.Fatalf("expected WeaklyDetectable, got %v (SNR=%.4f, Υ=%.4f, 2Υ=%.4f)", region, snr, limit, 2*limit)
	}
}

func TestClassify_Undetectable(t *testing.T) {
	// Very low SNR: values near zero with high variance relative to mean.
	// {-1, 1, -1, 1}: mean=0, SNR=0 (zero mean case) => Undetectable.
	tensions := []float64{-1, 1, -1, 1}
	region := Classify(tensions, 0.05)
	if region != Undetectable {
		t.Fatalf("expected Undetectable, got %v", region)
	}
}

func TestClassify_EmptyTensions(t *testing.T) {
	region := Classify(nil, 0.05)
	if region != Undetectable {
		t.Fatalf("expected Undetectable for nil tensions, got %v", region)
	}

	region = Classify([]float64{}, 0.05)
	if region != Undetectable {
		t.Fatalf("expected Undetectable for empty tensions, got %v", region)
	}
}

func TestDetectability_FullResult(t *testing.T) {
	// Use tensions that are strongly detectable.
	tensions := make([]float64, 50)
	for i := range tensions {
		tensions[i] = 5.0
	}
	result := Detectability(tensions, 0.05)

	if result.SNR <= 0 {
		t.Fatalf("expected SNR > 0, got %f", result.SNR)
	}
	if result.Threshold <= 0 {
		t.Fatalf("expected Threshold > 0, got %f", result.Threshold)
	}
	if result.Alpha != 0.05 {
		t.Fatalf("expected Alpha == 0.05, got %f", result.Alpha)
	}
	if result.Region != StronglyDetectable {
		t.Fatalf("expected StronglyDetectable region, got %v", result.Region)
	}
}

func TestCPS_BelowThreshold(t *testing.T) {
	// Tensions with zero mean => SNR=0 => not detectable => CPS=0.
	tensions := []float64{-1, 1, -1, 1}
	cps := CPS(tensions, 10.0, 0.05)
	if cps != 0 {
		t.Fatalf("expected CPS == 0 for undetectable tensions, got %f", cps)
	}
}

func TestCPS_AboveThreshold(t *testing.T) {
	// Strongly detectable tensions with positive mean and positive concealment cost.
	tensions := make([]float64, 100)
	for i := range tensions {
		tensions[i] = 5.0
	}
	cps := CPS(tensions, 10.0, 0.05)
	if cps <= 0 {
		t.Fatalf("expected CPS > 0, got %f", cps)
	}
	if cps >= 1.0 {
		t.Fatalf("expected CPS < 1, got %f", cps)
	}
}

func TestCPS_HighConcealment(t *testing.T) {
	// Higher concealment cost should yield higher CPS.
	tensions := make([]float64, 100)
	for i := range tensions {
		tensions[i] = 5.0
	}
	alpha := 0.05
	cpsLow := CPS(tensions, 1.0, alpha)
	cpsHigh := CPS(tensions, 100.0, alpha)

	if cpsLow >= cpsHigh {
		t.Fatalf("expected CPS(cost=1.0)=%f < CPS(cost=100.0)=%f", cpsLow, cpsHigh)
	}
}

func TestCPS_ZeroCost(t *testing.T) {
	tensions := make([]float64, 100)
	for i := range tensions {
		tensions[i] = 5.0
	}
	cps := CPS(tensions, 0, 0.05)
	if cps != 0 {
		t.Fatalf("expected CPS == 0 for zero concealment cost, got %f", cps)
	}
}

func TestDetectabilityRegion_String(t *testing.T) {
	tests := []struct {
		region   DetectabilityRegion
		expected string
	}{
		{Undetectable, "Undetectable"},
		{WeaklyDetectable, "WeaklyDetectable"},
		{StronglyDetectable, "StronglyDetectable"},
	}

	for _, tt := range tests {
		got := tt.region.String()
		if got != tt.expected {
			t.Fatalf("expected %q, got %q", tt.expected, got)
		}
	}

	// Unknown value should produce a fallback.
	unknown := DetectabilityRegion(99)
	s := unknown.String()
	if s == "" || s == "Undetectable" || s == "WeaklyDetectable" || s == "StronglyDetectable" {
		t.Fatalf("expected fallback string for unknown region, got %q", s)
	}
}

func TestDetectability_EmptyTensions(t *testing.T) {
	result := Detectability(nil, 0.05)
	if result.SNR != 0 {
		t.Fatalf("expected SNR == 0 for nil tensions, got %f", result.SNR)
	}
	if result.Region != Undetectable {
		t.Fatalf("expected Undetectable for nil tensions, got %v", result.Region)
	}
}

func TestCPS_Monotonic(t *testing.T) {
	// CPS should increase monotonically with concealment cost.
	tensions := make([]float64, 100)
	for i := range tensions {
		tensions[i] = 5.0
	}

	prev := 0.0
	for _, cost := range []float64{0.1, 1.0, 5.0, 10.0, 50.0, 100.0} {
		cps := CPS(tensions, cost, 0.05)
		if cps < prev {
			t.Fatalf("CPS should be monotonically increasing with cost; at cost=%f got CPS=%f < prev=%f", cost, cps, prev)
		}
		prev = cps
	}
}

func TestCPS_BoundedByOne(t *testing.T) {
	// CPS uses sigmoid normalization: result is in [0, 1).
	// With extremely large cost, floating-point rounds to 1.0, so we check <= 1.
	tensions := make([]float64, 100)
	for i := range tensions {
		tensions[i] = 5.0
	}
	cps := CPS(tensions, 1e12, 0.05)
	if cps > 1.0 {
		t.Fatalf("expected CPS <= 1 even for huge cost, got %f", cps)
	}
	if math.IsNaN(cps) || math.IsInf(cps, 0) {
		t.Fatalf("CPS should not be NaN or Inf, got %f", cps)
	}
	// With a moderate large cost, CPS should be close to but not exceed 1.
	cps2 := CPS(tensions, 100.0, 0.05)
	if cps2 <= 0 || cps2 > 1.0 {
		t.Fatalf("expected 0 < CPS <= 1 for large cost, got %f", cps2)
	}
}
