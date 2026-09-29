package harness

import (
	"math"
	"math/rand/v2"
	"reflect"
	"testing"
)

func TestGenerate_SameSeedSameDataset(t *testing.T) {
	cfg := DefaultConfig()
	for _, s := range Scenarios {
		a := Generate(s, cfg, 7)
		b := Generate(s, cfg, 7)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("%s: same seed produced different datasets", s)
		}
	}
	if reflect.DeepEqual(Generate(S1Removal, cfg, 7), Generate(S1Removal, cfg, 8)) {
		t.Fatal("different seeds produced the same dataset")
	}
}

func TestGenerate_S1TargetSilentAfterT0(t *testing.T) {
	cfg := DefaultConfig()
	for seed := uint64(1); seed <= 5; seed++ {
		d := Generate(S1Removal, cfg, seed)
		if d.Target == "" {
			t.Fatalf("seed %d: S1 has no target", seed)
		}
		before, after := 0, 0
		for w, counts := range d.Counts {
			for key, c := range counts {
				if key[0] != d.Target && key[1] != d.Target {
					continue
				}
				if w < d.T0 {
					before += c
				} else {
					after += c
				}
			}
		}
		if before == 0 {
			t.Fatalf("seed %d: target %s has no events before t0", seed, d.Target)
		}
		if after != 0 {
			t.Fatalf("seed %d: target %s has %d events after t0, want 0", seed, d.Target, after)
		}
	}
}

func TestGenerate_S0MeanCountMatchesMeanRate(t *testing.T) {
	d := Generate(S0Null, DefaultConfig(), 3)
	if d.Target != "" {
		t.Fatalf("S0 has target %q, want none", d.Target)
	}
	rateSum := 0.0
	for _, r := range d.Rates {
		rateSum += r
	}
	countSum := 0
	for _, counts := range d.Counts {
		for _, c := range counts {
			countSum += c
		}
	}
	meanRate := rateSum / float64(len(d.Rates))
	meanCount := float64(countSum) / float64(len(d.Rates)*d.T)
	if math.Abs(meanCount-meanRate)/meanRate > 0.05 {
		t.Fatalf("mean count %.3f, mean rate %.3f: differ by more than 5%%", meanCount, meanRate)
	}
}

func TestGenerate_Structure(t *testing.T) {
	cfg := DefaultConfig()
	d := Generate(S0Null, cfg, 11)
	if d.N != cfg.N || len(d.Nodes) != cfg.N {
		t.Fatalf("N = %d, len(Nodes) = %d, want %d", d.N, len(d.Nodes), cfg.N)
	}
	outDeg := map[string]int{}
	for key := range d.Rates {
		if key[0] == key[1] {
			t.Fatalf("self-loop on %s", key[0])
		}
		outDeg[key[0]]++
	}
	for _, id := range d.Nodes {
		if deg := outDeg[id]; deg < 2 || deg > cfg.MaxOutDegree {
			t.Fatalf("node %s has out-degree %d, want [2, %d]", id, deg, cfg.MaxOutDegree)
		}
	}
}

func TestGenerate_S3PlantsSniper(t *testing.T) {
	cfg := DefaultConfig()
	d := Generate(S3Sniper, cfg, 5)
	if d.N != cfg.N+1 || d.Target != d.Nodes[cfg.N] {
		t.Fatalf("expected planted node %s as target, got N=%d target=%q", nodeID(cfg.N), d.N, d.Target)
	}
	out, in := 0, 0
	for key := range d.Rates {
		if key[0] == d.Target {
			out++
		}
		if key[1] == d.Target {
			in++
		}
	}
	if out < 2 || out > 3 || in != 0 {
		t.Fatalf("sniper has out=%d in=%d, want out in [2,3] and in=0", out, in)
	}
	for w := d.T0; w < d.T; w++ {
		for key := range d.Counts[w] {
			if key[0] == d.Target {
				t.Fatalf("sniper active in window %d", w)
			}
		}
	}
}

func TestGenerate_S4TargetHasLowestDegree(t *testing.T) {
	d := Generate(S4Hermit, DefaultConfig(), 9)
	deg := map[string]int{}
	for key := range d.Rates {
		deg[key[0]]++
		deg[key[1]]++
	}
	for _, id := range d.Nodes {
		if deg[id] < deg[d.Target] {
			t.Fatalf("node %s has degree %d < hermit %s degree %d", id, deg[id], d.Target, deg[d.Target])
		}
	}
}

func TestGenerate_BeforeWindowsSharedAcrossScenarios(t *testing.T) {
	// Common random numbers: the before period of S1 equals the null's.
	cfg := DefaultConfig()
	null := Generate(S0Null, cfg, 4)
	s1 := Generate(S1Removal, cfg, 4)
	for w := 0; w < cfg.T0; w++ {
		if !reflect.DeepEqual(null.Counts[w], s1.Counts[w]) {
			t.Fatalf("window %d differs between S0 and S1", w)
		}
	}
}

func TestPoisson_Mean(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	for _, lambda := range []float64{0.5, 4, 45, 120} {
		const n = 20000
		sum := 0
		for i := 0; i < n; i++ {
			sum += poisson(rng, lambda)
		}
		mean := float64(sum) / n
		if math.Abs(mean-lambda) > 4*math.Sqrt(lambda/n) {
			t.Fatalf("lambda=%.1f: sample mean %.3f", lambda, mean)
		}
	}
}
