package harness

import (
	"math"
	"testing"
)

func TestGlobalQ(t *testing.T) {
	before := map[[2]string]int{{"a", "b"}: 30, {"b", "c"}: 10}
	after := map[[2]string]int{{"a", "b"}: 20}
	if q := globalQ(before, after, 0.25); math.Abs(q-20.0/60) > 1e-12 {
		t.Fatalf("q̂ = %v, want 1/3", q)
	}
	if q := globalQ(before, nil, 0.25); q != 0.25 {
		t.Fatalf("no events after: q̂ = %v, want the fallback 0.25", q)
	}
}

func TestEdgeZ_GlobalQAbsorbsUniformTrend(t *testing.T) {
	// Every edge doubles its rate after T0: with the fixed q every node looks
	// like a burst; with q̂ nobody does.
	edges := map[[2]string][]int{}
	nodes := []string{"a", "b", "c", "d"}
	for i, u := range nodes {
		for _, v := range nodes[i+1:] {
			edges[[2]string{u, v}] = []int{10, 10, 20}
		}
	}
	d := tinyDataset(nodes, 3, 2, "", edges)
	fixed := EdgeZ(d, 2, DefaultZConfig())
	cfg := DefaultZConfig()
	cfg.GlobalQ = true
	adjusted := EdgeZ(d, 2, cfg)
	for _, id := range nodes {
		if fixed[id] < 2 {
			t.Errorf("fixed q: Z(%s) = %v, want a clear burst", id, fixed[id])
		}
		if math.Abs(adjusted[id]) > 0.5 {
			t.Errorf("q̂: Z(%s) = %v, want ≈ 0", id, adjusted[id])
		}
	}
}

func TestEstimatePhi(t *testing.T) {
	cfg := DefaultConfig()
	d := Generate(S0Null, cfg, 1)
	if p := estimatePhi(d, d.T0, PhiMedian)([2]string{}); p != 1 {
		t.Errorf("Poisson: φ̂ = %v, want 1 (the median Pearson is below 1, floored)", p)
	}
	cfg.Dispersion = 1
	d = Generate(S0Null, cfg, 1)
	global := estimatePhi(d, d.T0, PhiMedian)([2]string{})
	if global < 2 {
		t.Errorf("k = 1: φ̂ = %v, want well above 1", global)
	}
	// Under gamma-Poisson φ = 1 + λ/k grows with the rate, and so do the bands.
	phi := estimatePhi(d, d.T0, PhiBands)
	var low, high [2]string
	for key := range d.Rates {
		mean := float64(sumCounts(d, 0, d.T0)[key]) / float64(d.T0)
		if mean >= 1 && mean < 2 && low == ([2]string{}) {
			low = key
		}
		if mean >= 16 && high == ([2]string{}) {
			high = key
		}
	}
	if low == ([2]string{}) || high == ([2]string{}) {
		t.Skip("no edges in the extreme bands")
	}
	if phi(low) >= phi(high) {
		t.Errorf("φ̂ bands: low-rate %v should be below high-rate %v", phi(low), phi(high))
	}
}
