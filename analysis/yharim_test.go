package analysis

import (
	"math"
	"testing"
)

func TestYharimLimit_StandardAlpha(t *testing.T) {
	tests := []struct {
		alpha    float64
		expected float64
		tol      float64
	}{
		{0.05, 2.448, 0.001},
		{0.01, 3.035, 0.001},
		{0.001, 3.717, 0.001},
	}

	for _, tt := range tests {
		got := YharimLimit(tt.alpha)
		if math.Abs(got-tt.expected) > tt.tol {
			t.Errorf("YharimLimit(%f) = %f, want ~%f (tol %f)", tt.alpha, got, tt.expected, tt.tol)
		}
	}
}

func TestYharimLimit_InvalidAlpha(t *testing.T) {
	// alpha = 0 -> +Inf
	if v := YharimLimit(0); !math.IsInf(v, 1) {
		t.Errorf("YharimLimit(0) = %f, want +Inf", v)
	}

	// alpha = 1 -> 0
	if v := YharimLimit(1); v != 0 {
		t.Errorf("YharimLimit(1) = %f, want 0", v)
	}

	// alpha < 0 -> +Inf
	if v := YharimLimit(-0.5); !math.IsInf(v, 1) {
		t.Errorf("YharimLimit(-0.5) = %f, want +Inf", v)
	}
}

func TestYharimLimit_BoundaryValues(t *testing.T) {
	// Very small alpha -> large threshold
	v := YharimLimit(1e-10)
	if v <= 0 || math.IsNaN(v) {
		t.Errorf("YharimLimit(1e-10) = %f, want positive finite", v)
	}

	// alpha just below 1
	v = YharimLimit(0.999)
	if v < 0 || math.IsNaN(v) {
		t.Errorf("YharimLimit(0.999) = %f, want non-negative", v)
	}
}

func TestSNR_SingleValue(t *testing.T) {
	// Single value: stddev = 0, mean > 0 -> +Inf
	v := SNR([]float64{5.0})
	if !math.IsInf(v, 1) {
		t.Errorf("SNR([5.0]) = %f, want +Inf", v)
	}
}

func TestSNR_SingleValueZero(t *testing.T) {
	// Single value of 0: mean = 0 -> SNR = 0
	v := SNR([]float64{0.0})
	if v != 0 {
		t.Errorf("SNR([0.0]) = %f, want 0", v)
	}
}

func TestSNR_IdenticalValues(t *testing.T) {
	// All same nonzero values -> stddev = 0, mean > 0 -> +Inf
	v := SNR([]float64{3.0, 3.0, 3.0, 3.0})
	if !math.IsInf(v, 1) {
		t.Errorf("SNR([3,3,3,3]) = %f, want +Inf", v)
	}
}

func TestSNR_EmptySlice(t *testing.T) {
	v := SNR([]float64{})
	if v != 0 {
		t.Errorf("SNR([]) = %f, want 0", v)
	}
}

func TestSNR_MixedValues(t *testing.T) {
	// Known values: [2, 4, 6, 8]
	// mean = 5, variance = ((2-5)^2 + (4-5)^2 + (6-5)^2 + (8-5)^2) / 4
	//         = (9 + 1 + 1 + 9) / 4 = 20/4 = 5
	// stddev = sqrt(5) ~ 2.2361
	// SNR = (5 / sqrt(5)) * sqrt(4) = (5/2.2361) * 2 = 2.2361 * 2 = 4.4721
	tensions := []float64{2.0, 4.0, 6.0, 8.0}
	got := SNR(tensions)
	expected := (5.0 / math.Sqrt(5.0)) * math.Sqrt(4.0)

	if math.Abs(got-expected) > 1e-10 {
		t.Errorf("SNR([2,4,6,8]) = %f, want %f", got, expected)
	}
}

func TestSNR_AllZeros(t *testing.T) {
	// All zeros: mean = 0 -> SNR = 0
	v := SNR([]float64{0, 0, 0})
	if v != 0 {
		t.Errorf("SNR([0,0,0]) = %f, want 0", v)
	}
}

func TestIsDetectable_AboveThreshold(t *testing.T) {
	// Tensions with high SNR at alpha=0.05.
	// Yharim limit at alpha=0.05 is ~2.448.
	// All identical nonzero values -> SNR = +Inf -> detectable.
	tensions := []float64{5.0, 5.0, 5.0, 5.0}
	if !IsDetectable(tensions, 0.05) {
		t.Error("expected detectable for identical nonzero tensions at alpha=0.05")
	}
}

func TestIsDetectable_BelowThreshold(t *testing.T) {
	// Tensions with low SNR at alpha=0.05.
	// Yharim limit at alpha=0.05 is ~2.448.
	// We need SNR < 2.448.
	// Use values with high variance relative to mean.
	// [0.1, 10.0]: mean=5.05, variance=((0.1-5.05)^2+(10-5.05)^2)/2=24.5025
	// stddev=4.95, SNR=(5.05/4.95)*sqrt(2) = 1.0202*1.4142 = 1.4427 < 2.448
	tensions := []float64{0.1, 10.0}
	if IsDetectable(tensions, 0.05) {
		snr := SNR(tensions)
		limit := YharimLimit(0.05)
		t.Errorf("expected not detectable, SNR=%f, YharimLimit=%f", snr, limit)
	}
}

func TestIsDetectable_EmptyTensions(t *testing.T) {
	// Empty tensions -> SNR = 0 -> not detectable at any reasonable alpha.
	if IsDetectable([]float64{}, 0.05) {
		t.Error("expected not detectable for empty tensions")
	}
}

func TestIsDetectable_AlphaEdgeCases(t *testing.T) {
	tensions := []float64{5.0, 5.0, 5.0}

	// alpha = 0 -> YharimLimit = +Inf -> never detectable
	if IsDetectable(tensions, 0) {
		t.Error("expected not detectable at alpha=0")
	}

	// alpha = 1 -> YharimLimit = 0 -> always detectable (if SNR > 0)
	if !IsDetectable(tensions, 1.0) {
		t.Error("expected detectable at alpha=1.0")
	}
}
