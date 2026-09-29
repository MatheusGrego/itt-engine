package harness

import (
	"math"
	"testing"
)

func TestAUCSet(t *testing.T) {
	nodes := []string{"a", "b", "c", "d"}
	s := map[string]float64{"a": 4, "b": 3, "c": 1, "d": 1}
	if got := AUCSet(s, nodes, []string{"a", "b"}); got != 1 {
		t.Fatalf("AUCSet = %v, want 1", got)
	}
	if got := AUCSet(s, nodes, []string{"c"}); math.Abs(got-0.5/3) > 1e-12 {
		t.Fatalf("AUCSet = %v, want 1/6 (one tie among three others)", got)
	}
}

func TestGenerateRelay_LatentAndDrop(t *testing.T) {
	cfg := DefaultConfig()
	rd := GenerateRelay(cfg, 3, 1, true)
	for _, id := range rd.Nodes {
		if id == rd.Relay {
			t.Fatal("relay is in Nodes")
		}
	}
	for w := range rd.Counts {
		for key := range rd.Counts[w] {
			if key[0] == rd.Relay || key[1] == rd.Relay {
				t.Fatalf("edge %v of the latent relay is in the data", key)
			}
		}
	}
	if len(rd.Neighbors) == 0 || rd.Flows == 0 {
		t.Fatalf("relay %s has %d neighbours and %d flows", rd.Relay, len(rd.Neighbors), rd.Flows)
	}
	// ρ = 1 with drop: the relayed part vanishes, so after T0 the flow edges
	// only keep what the base graph already had between the same nodes.
	base := GenerateRelay(cfg, 3, 1, false)
	after := func(d RelayDataset) int {
		s := 0
		for w := d.T0; w < d.T; w++ {
			for _, c := range d.Counts[w] {
				s += c
			}
		}
		return s
	}
	if after(rd) >= after(base) {
		t.Fatalf("drop did not lower the after total: %d vs %d", after(rd), after(base))
	}
}
