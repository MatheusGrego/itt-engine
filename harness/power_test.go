package harness

import (
	"math"
	"testing"
)

func TestGenerate_S2xMultBandAndPartial(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ThinMult = 0
	cfg.TargetLo, cfg.TargetHi = 0.8, 0.95
	d := Generate(S2xParam, cfg, 4)
	if len(d.Altered) == 0 {
		t.Fatal("S2x: no altered edges")
	}
	for _, key := range d.Altered {
		for w := d.T0; w < d.T; w++ {
			if d.Counts[w][key] != 0 {
				t.Fatalf("mult 0: altered edge %v has events after T0", key)
			}
		}
	}
	full := len(d.Altered)

	cfg.PartialFrac = 0.5
	p := Generate(S2xParam, cfg, 4)
	if p.Target != d.Target {
		t.Fatalf("partial run picked another target: %s vs %s", p.Target, d.Target)
	}
	if want := int(math.Round(0.5 * float64(full))); len(p.Altered) != want {
		t.Fatalf("PartialFrac 0.5: %d altered edges, want %d of %d", len(p.Altered), want, full)
	}

	// A high-band target has more edges than a low-band one.
	cfg.PartialFrac = 0
	cfg.TargetLo, cfg.TargetHi = 0.1, 0.3
	if low := Generate(S2xParam, cfg, 4); len(low.Altered) >= full {
		t.Fatalf("low band target has %d edges, high band %d", len(low.Altered), full)
	}
}

func TestPredictPower(t *testing.T) {
	// No change: power is the per-node level P(N(0,1) > zc).
	p, h := predictPower([]float64{4, 4}, 2, 1, 4, 8, 1.645)
	if h != 0 || math.Abs(p-0.05) > 1e-3 {
		t.Fatalf("mult 1: power %v hellinger %v, want 0.05 and 0", p, h)
	}
	// Removal of strong edges: power ≈ 1.
	if p, _ := predictPower([]float64{10, 10, 10}, 3, 0, 4, 8, 3.72); p < 0.99 {
		t.Fatalf("removal: power %v, want ≈ 1", p)
	}
}
