package harness

import (
	"math"
	"testing"
)

func TestScanSplits(t *testing.T) {
	if got := ScanSplits(12); len(got) != 4 || got[0] != 8 || got[3] != 11 {
		t.Fatalf("ScanSplits(12) = %v, want [8 9 10 11]", got)
	}
	if got := ScanSplits(3); len(got) != 2 || got[0] != 1 {
		t.Fatalf("ScanSplits(3) = %v, want [1 2]", got)
	}
}

func TestGenerate_T0Random(t *testing.T) {
	cfg := DefaultConfig()
	cfg.T0Random = true
	seen := map[int]bool{}
	for seed := uint64(1); seed <= 40; seed++ {
		d := Generate(S1Removal, cfg, seed)
		if d.T0 < 8 || d.T0 > 11 {
			t.Fatalf("seed %d: t0 = %d outside [8, 11]", seed, d.T0)
		}
		seen[d.T0] = true
		for w := d.T0; w < d.T; w++ {
			for key, c := range d.Counts[w] {
				if (key[0] == d.Target || key[1] == d.Target) && c > 0 {
					t.Fatalf("seed %d: target has events after its t0 = %d", seed, d.T0)
				}
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("t0 values drawn: %v, want all of 8..11", seen)
	}
}

func TestScanZ_FindsUnknownSplit(t *testing.T) {
	// v's edges go quiet at window 10 of 12; the scan must beat the fixed
	// split at 8, which mixes quiet and active windows.
	d := tinyDataset([]string{"v", "a", "b"}, 12, 8, "v", map[[2]string][]int{
		{"a", "v"}: {6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 0, 0},
		{"b", "v"}: {6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 0, 0},
		{"a", "b"}: {6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6},
	})
	scan := ScanZ{Cfg: DefaultZConfig(), T: 12}.Scores(d)
	fixed := SilenceZ{Cfg: DefaultZConfig()}.Scores(d)
	at10 := -EdgeZ(d, 10, DefaultZConfig())["v"]
	if math.Abs(scan["v"]-at10) > 1e-12 {
		t.Fatalf("scan(v) = %v, want the split-10 value %v", scan["v"], at10)
	}
	if scan["v"] <= fixed["v"] {
		t.Fatalf("scan(v) = %v should exceed the fixed-split score %v", scan["v"], fixed["v"])
	}
	// Bonferroni over N·4 is stricter than over N.
	if !(SilenceZ{}).Alarm(3.8, 500, 0.05) || (ScanZ{T: 12}).Alarm(3.8, 500, 0.05) {
		t.Fatal("3.8 should alarm at z(1−α/N) but not at z(1−α/(4N)) for N = 500")
	}
}
