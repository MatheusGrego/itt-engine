package harness

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestSignedRoot_NullIsStandardNormalForLargeCounts(t *testing.T) {
	// n ~ Poisson(50) per edge, a third of it expected after the split.
	const lambda, q, samples = 50.0, 1.0 / 3, 10000
	rng := rand.New(rand.NewPCG(1, 2))
	var sum, sum2 float64
	for i := 0; i < samples; i++ {
		a := float64(poisson(rng, lambda*(1-q)))
		o := float64(poisson(rng, lambda*q))
		r := SignedRoot(a, o, q)
		sum += r
		sum2 += r * r
	}
	mean := sum / samples
	variance := sum2/samples - mean*mean
	// Standard errors are ~0.01 for the mean and ~0.014 for the variance;
	// the signed root also has a small O(1/sqrt(n)) negative bias.
	if math.Abs(mean) > 0.05 {
		t.Errorf("mean(r) = %.4f, want ≈ 0", mean)
	}
	if math.Abs(variance-1) > 0.06 {
		t.Errorf("var(r) = %.4f, want ≈ 1", variance)
	}
}

func TestSignedRoot_EdgeCases(t *testing.T) {
	const q = 1.0 / 3
	if r := SignedRoot(0, 5, q); math.IsInf(r, 0) || math.IsNaN(r) || r <= 0 {
		t.Errorf("new edge (a = 0, o = 5): r = %v, want finite and > 0", r)
	}
	if r := SignedRoot(12, 0, q); math.IsInf(r, 0) || math.IsNaN(r) || r >= 0 {
		t.Errorf("edge gone quiet (a = 12, o = 0): r = %v, want finite and < 0", r)
	}
	if r := SignedRoot(0, 0, q); r != 0 {
		t.Errorf("no events: r = %v, want 0", r)
	}
	// o exactly as expected: G = 0.
	if r := SignedRoot(8, 4, q); math.Abs(r) > 1e-12 {
		t.Errorf("o = ô: r = %v, want 0", r)
	}
	// Closed form for o = 0: G = 2 a ln(1/(1−q)).
	if r, want := SignedRoot(12, 0, q), -math.Sqrt(2*12*math.Log(1/(1-q))); math.Abs(r-want) > 1e-12 {
		t.Errorf("a = 12, o = 0: r = %v, want %v", r, want)
	}
}

func TestTwoPoissonZ_StoufferAndIsolatedNode(t *testing.T) {
	// T = 3, T0 = 2, q = 1/3. a->b goes quiet, c->b appears only after,
	// iso has no edge at all.
	d := tinyDataset([]string{"a", "b", "c", "iso"}, 3, 2, "", map[[2]string][]int{
		{"a", "b"}: {6, 6, 0},
		{"c", "b"}: {0, 0, 3},
	})
	z := TwoPoissonZ(d)
	q := 1.0 / 3
	rAB, rCB := SignedRoot(12, 0, q), SignedRoot(0, 3, q)
	want := map[string]float64{
		"a":   rAB,
		"b":   (rAB + rCB) / math.Sqrt2,
		"c":   rCB,
		"iso": 0,
	}
	for id, w := range want {
		got, ok := z[id]
		if !ok {
			t.Fatalf("Z(%s) missing", id)
		}
		if math.Abs(got-w) > 1e-12 {
			t.Errorf("Z(%s) = %v, want %v", id, got, w)
		}
	}
	if z["a"] >= 0 || z["c"] <= 0 {
		t.Errorf("Z(a) = %v should be < 0 (silence), Z(c) = %v should be > 0 (burst)", z["a"], z["c"])
	}
	if s := (SilenceZ{}).Scores(d); s["a"] != -z["a"] {
		t.Errorf("SilenceZ(a) = %v, want %v", s["a"], -z["a"])
	}
}

func TestZCritical(t *testing.T) {
	// α/N = 0.025 → z = 1.959964; α/N = 1e-4 → z = 3.719016.
	cases := []struct {
		n     int
		alpha float64
		want  float64
	}{{2, 0.05, 1.959964}, {500, 0.05, 3.719016}}
	for _, c := range cases {
		if got := zCritical(c.n, c.alpha); math.Abs(got-c.want) > 1e-5 {
			t.Errorf("zCritical(%d, %v) = %v, want %v", c.n, c.alpha, got, c.want)
		}
	}
	if (SilenceZ{}).Alarm(3.7, 500, 0.05) || !(SilenceZ{}).Alarm(3.72, 500, 0.05) {
		t.Error("SilenceZ alarm should switch between 3.70 and 3.72 at N = 500, α = 0.05")
	}
}
