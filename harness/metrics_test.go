package harness

import (
	"fmt"
	"math"
	"testing"
)

func testNodes(n int) []string {
	nodes := make([]string, n)
	for i := range nodes {
		nodes[i] = fmt.Sprintf("x%d", i)
	}
	return nodes
}

func TestAUC_PerfectScoreIsOne(t *testing.T) {
	nodes := testNodes(50)
	scores := map[string]float64{}
	for i, id := range nodes {
		scores[id] = float64(i)
	}
	if got := AUC(scores, nodes, nodes[49]); got != 1 {
		t.Fatalf("AUC = %v, want 1", got)
	}
	if got := HitAtK(scores, nodes, nodes[49], 10); got != 1 {
		t.Fatalf("hit@10 = %v, want 1", got)
	}
}

func TestAUC_ConstantScoreIsHalf(t *testing.T) {
	nodes := testNodes(100)
	scores := map[string]float64{}
	for _, id := range nodes {
		scores[id] = 3.7
	}
	if got := AUC(scores, nodes, nodes[10]); got != 0.5 {
		t.Fatalf("AUC = %v, want 0.5", got)
	}
	// Random tie-breaking over 100 nodes puts the target in the top 10 with p = 0.1.
	if got := HitAtK(scores, nodes, nodes[10], 10); math.Abs(got-0.1) > 1e-12 {
		t.Fatalf("hit@10 = %v, want 0.1", got)
	}
}

func TestAUC_WorstScoreIsZero(t *testing.T) {
	nodes := testNodes(20)
	scores := map[string]float64{}
	for i, id := range nodes {
		scores[id] = float64(i)
	}
	if got := AUC(scores, nodes, nodes[0]); got != 0 {
		t.Fatalf("AUC = %v, want 0", got)
	}
	if got := HitAtK(scores, nodes, nodes[0], 10); got != 0 {
		t.Fatalf("hit@10 = %v, want 0", got)
	}
}

func TestRun_Smoke(t *testing.T) {
	opts := DefaultOptions()
	opts.Config.N = 80
	opts.Replicas = 3
	results := Run(DefaultMethods(), opts)
	if want := len(Scenarios) * len(DefaultMethods()); len(results) != want {
		t.Fatalf("got %d results, want %d", len(results), want)
	}
	for _, r := range results {
		if r.Scenario == S0Null {
			if !math.IsNaN(r.AUCMean) {
				t.Fatalf("S0 %s: AUC should be NaN, got %v", r.Method, r.AUCMean)
			}
			if r.FWER < 0 || r.FWER > 1 {
				t.Fatalf("S0 %s: FWER = %v", r.Method, r.FWER)
			}
			continue
		}
		if r.AUCMean < 0 || r.AUCMean > 1 || r.HitAtK < 0 || r.HitAtK > 1 {
			t.Fatalf("%s %s: AUC = %v hit = %v out of [0, 1]", r.Scenario, r.Method, r.AUCMean, r.HitAtK)
		}
	}
}

func TestWilson(t *testing.T) {
	// 14 of 200: p = 0.07, 95% Wilson interval ≈ [0.0422, 0.1140].
	lo, hi := Wilson(14, 200)
	if math.Abs(lo-0.0422) > 5e-4 || math.Abs(hi-0.1140) > 5e-4 {
		t.Fatalf("Wilson(14, 200) = [%.4f, %.4f], want ≈ [0.0422, 0.1140]", lo, hi)
	}
	if lo, _ := Wilson(0, 100); lo != 0 {
		t.Fatalf("Wilson(0, 100) lower = %v, want 0", lo)
	}
}
