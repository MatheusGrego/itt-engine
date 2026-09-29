package harness

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestProbitLog_KnownQuantiles(t *testing.T) {
	cases := []struct{ p, want float64 }{
		{0.025, -1.959963984540054},
		{0.975, 1.959963984540054},
		{1e-10, -6.361340902404056},
		{0.5, 0},
	}
	for _, c := range cases {
		if got := probitLog(math.Log(c.p)); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("Φ⁻¹(%g) = %.15f, want %.15f", c.p, got, c.want)
		}
	}
	// Agrees with the stdlib where the stdlib is accurate.
	for _, p := range []float64{1e-8, 1e-4, 0.01, 0.2, 0.4999} {
		want := -math.Sqrt2 * math.Erfcinv(2*p)
		if got := probitLog(math.Log(p)); math.Abs(got-want) > 1e-7 {
			t.Errorf("Φ⁻¹(%g) = %.12f, stdlib %.12f", p, got, want)
		}
	}
}

func TestProbitLog_FarTailRoundTrip(t *testing.T) {
	// Far below float64: ln p = −1e5 is p ≈ 10^−43429.
	for _, lp := range []float64{-1, -5, -50, -700, -800, -5000, -1e5} {
		z := probitLog(lp)
		if math.IsInf(z, 0) || math.IsNaN(z) {
			t.Fatalf("probitLog(%g) = %v", lp, z)
		}
		if got := lnPhi(z); math.Abs(got-lp) > 1e-9*math.Abs(lp) {
			t.Errorf("lnPhi(probitLog(%g)) = %v", lp, got)
		}
	}
}

func TestMidPZ_SmallNExact(t *testing.T) {
	const q = 1.0 / 3
	// n = 1: o = 0 has lower mid-p (2/3)/2 = 1/3; o = 1 has 2/3 + (1/3)/2 = 5/6.
	cases := []struct{ a, o, p float64 }{{1, 0, 1.0 / 3}, {0, 1, 5.0 / 6}}
	for _, c := range cases {
		want := probitLog(math.Log(c.p))
		if got := MidPZ(c.a, c.o, q); math.Abs(got-want) > 1e-12 {
			t.Errorf("MidPZ(%v, %v) = %v, want %v", c.a, c.o, got, want)
		}
	}
	// n = 3, o = 1 = nq: lower mid-p = 8/27 + (12/27)/2 = 14/27.
	if got, want := MidPZ(2, 1, q), probitLog(math.Log(14.0/27)); math.Abs(got-want) > 1e-12 {
		t.Errorf("MidPZ(2, 1) = %v, want %v", got, want)
	}
}

func TestMidPZ_EdgeCasesAndTails(t *testing.T) {
	const q = 1.0 / 3
	if r := MidPZ(0, 5, q); math.IsInf(r, 0) || r <= 0 {
		t.Errorf("new edge: MidPZ = %v, want finite > 0", r)
	}
	if r := MidPZ(12, 0, q); math.IsInf(r, 0) || r >= 0 {
		t.Errorf("edge gone quiet: MidPZ = %v, want finite < 0", r)
	}
	if r := MidPZ(0, 0, q); r != 0 {
		t.Errorf("no events: MidPZ = %v, want 0", r)
	}
	// P(O = 0) = (2/3)^20000 ≈ e^−8109, far below float64.
	if r := MidPZ(20000, 0, q); math.IsInf(r, 0) || math.IsNaN(r) || r > -100 {
		t.Errorf("a = 20000, o = 0: MidPZ = %v, want finite and ≈ −127", r)
	}
	// For large n the exact and the asymptotic statistics agree.
	for _, c := range [][2]float64{{400, 200}, {430, 170}, {370, 230}} {
		if mp, sr := MidPZ(c[0], c[1], q), SignedRoot(c[0], c[1], q); math.Abs(mp-sr) > 0.05 {
			t.Errorf("a = %v, o = %v: mid-p %v vs signed root %v", c[0], c[1], mp, sr)
		}
	}
}

func TestMidPZ_NullIsStandardNormalForLargeCounts(t *testing.T) {
	const lambda, q, samples = 50.0, 1.0 / 3, 10000
	rng := rand.New(rand.NewPCG(1, 2))
	var sum, sum2 float64
	for i := 0; i < samples; i++ {
		a := float64(poisson(rng, lambda*(1-q)))
		o := float64(poisson(rng, lambda*q))
		r := MidPZ(a, o, q)
		sum += r
		sum2 += r * r
	}
	mean := sum / samples
	variance := sum2/samples - mean*mean
	if math.Abs(mean) > 0.05 {
		t.Errorf("mean(r) = %.4f, want ≈ 0", mean)
	}
	// The mid-p quantile is slightly under-dispersed for discrete data.
	if math.Abs(variance-1) > 0.06 {
		t.Errorf("var(r) = %.4f, want ≈ 1", variance)
	}
}
