package harness

import (
	"math"
	"math/rand/v2"
	"reflect"
	"testing"
)

func TestGammaRand_MeanAndVariance(t *testing.T) {
	for _, shape := range []float64{0.5, 1, 3, 10} {
		rng := rand.New(rand.NewPCG(5, uint64(shape*10)))
		const n = 200000
		var sum, sum2 float64
		for i := 0; i < n; i++ {
			x := gammaRand(rng, shape)
			sum += x
			sum2 += x * x
		}
		mean := sum / n
		variance := sum2/n - mean*mean
		// Gamma(k, 1): mean = var = k.
		if math.Abs(mean-shape) > 0.02*shape+0.01 || math.Abs(variance-shape) > 0.05*shape+0.01 {
			t.Errorf("Gamma(%v): mean %.4f var %.4f, want %v and %v", shape, mean, variance, shape, shape)
		}
	}
}

func TestGenerate_StressZeroValuesKeepDefault(t *testing.T) {
	cfg := DefaultConfig()
	explicit := cfg
	explicit.Dispersion, explicit.Trend, explicit.Seasonality, explicit.Drift = 0, 0, 0, 0
	explicit.SeasonPeriod = 4
	if !reflect.DeepEqual(Generate(S2Thinning, cfg, 11), Generate(S2Thinning, explicit, 11)) {
		t.Fatal("zero stress options changed the dataset")
	}
	for w := 0; w < cfg.T; w++ {
		if m := cfg.windowMult(w); m != 1 {
			t.Fatalf("windowMult(%d) = %v, want exactly 1", w, m)
		}
	}
}

// windowTotals returns the total count per window, summed over replicas.
func windowTotals(cfg Config, replicas int) []float64 {
	tot := make([]float64, cfg.T)
	for r := 1; r <= replicas; r++ {
		d := Generate(S0Null, cfg, uint64(r))
		for w, counts := range d.Counts {
			for _, c := range counts {
				tot[w] += float64(c)
			}
		}
	}
	return tot
}

func TestGenerate_TrendAndSeasonalityShapeWindowTotals(t *testing.T) {
	cfg := DefaultConfig()
	cfg.N = 200
	base := windowTotals(cfg, 3)

	trend := cfg
	trend.Trend = 0.15
	tt := windowTotals(trend, 3)
	for w := range tt {
		if ratio, want := tt[w]/base[w], math.Pow(1.15, float64(w)); math.Abs(ratio/want-1) > 0.05 {
			t.Errorf("trend: window %d ratio %.3f, want %.3f", w, ratio, want)
		}
	}

	season := cfg
	season.Seasonality = 0.3
	st := windowTotals(season, 3)
	for w := range st {
		want := 1 + 0.3*math.Sin(2*math.Pi*float64(w)/4)
		if ratio := st[w] / base[w]; math.Abs(ratio/want-1) > 0.05 {
			t.Errorf("season: window %d ratio %.3f, want %.3f", w, ratio, want)
		}
	}
}

func TestGenerate_DispersionInflatesVariance(t *testing.T) {
	// Pearson dispersion of the per-window counts of one edge, averaged over
	// edges with a similar rate: ≈ 1 for Poisson, ≈ 1 + λ/k for gamma-Poisson.
	measure := func(k float64) float64 {
		cfg := DefaultConfig()
		cfg.Dispersion = k
		var sum float64
		var n int
		for r := 1; r <= 3; r++ {
			d := Generate(S0Null, cfg, uint64(r))
			for key, rate := range d.Rates {
				if rate < 3 || rate > 5 {
					continue
				}
				var s, s2 float64
				for w := 0; w < d.T; w++ {
					c := float64(d.Counts[w][key])
					s += c
					s2 += c * c
				}
				mean := s / float64(d.T)
				variance := (s2 - float64(d.T)*mean*mean) / float64(d.T-1)
				sum += variance / rate
				n++
			}
		}
		return sum / float64(n)
	}
	if p := measure(0); math.Abs(p-1) > 0.1 {
		t.Errorf("Poisson: dispersion %.3f, want ≈ 1", p)
	}
	// λ ≈ 4: k = 1 gives ≈ 5, k = 10 gives ≈ 1.4.
	if p := measure(1); p < 4 || p > 6 {
		t.Errorf("k = 1: dispersion %.3f, want ≈ 5", p)
	}
	if p := measure(10); p < 1.2 || p > 1.6 {
		t.Errorf("k = 10: dispersion %.3f, want ≈ 1.4", p)
	}
}
