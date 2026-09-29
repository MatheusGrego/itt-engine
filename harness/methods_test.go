package harness

import (
	"math"
	"testing"
)

// tinyDataset builds a dataset from per-window counts of a few edges.
func tinyDataset(nodes []string, t, t0 int, target string, edges map[[2]string][]int) Dataset {
	d := Dataset{N: len(nodes), T: t, T0: t0, Nodes: nodes, Target: target, Counts: make([]map[[2]string]int, t)}
	for w := range d.Counts {
		d.Counts[w] = map[[2]string]int{}
	}
	for key, series := range edges {
		for w, c := range series {
			if c > 0 {
				d.Counts[w][key] = c
			}
		}
	}
	return d
}

func TestPoissonDeviance(t *testing.T) {
	cases := []struct{ o, e, want float64 }{
		{0, 3, 6},
		{4, 4, 0},
		{2, 1, 2 * (2*math.Log(2) - 1)},
	}
	for _, c := range cases {
		if got := PoissonDeviance(c.o, c.e); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("PoissonDeviance(%v, %v) = %v, want %v", c.o, c.e, got, c.want)
		}
	}
}

func TestDeviance_SilenceAndBurst(t *testing.T) {
	// T = 4, T0 = 2. a->b goes silent, c->b doubles, a->c appears only after.
	d := tinyDataset([]string{"a", "b", "c"}, 4, 2, "", map[[2]string][]int{
		{"a", "b"}: {3, 3, 0, 0},
		{"c", "b"}: {2, 2, 4, 4},
		{"a", "c"}: {0, 0, 5, 5},
	})
	s, b := Deviance(d)

	silentAB := PoissonDeviance(0, 6) // e = 3 * 2 = 6
	burstCB := PoissonDeviance(8, 4)  // e = 2 * 2 = 4
	want := map[string][2]float64{
		"a": {silentAB, 0},
		"b": {silentAB, burstCB},
		"c": {0, burstCB}, // a->c is new: left out of both sums
	}
	for id, w := range want {
		if math.Abs(s[id]-w[0]) > 1e-12 || math.Abs(b[id]-w[1]) > 1e-12 {
			t.Errorf("%s: S = %v B = %v, want S = %v B = %v", id, s[id], b[id], w[0], w[1])
		}
	}
}

func TestTauJSD_RemovedNodeScoresZeroAfterButNotCumulative(t *testing.T) {
	// v is the sole target of a and b; it disappears at T0. M1 only sees the
	// after windows, where v has no edges (T1); M2 still sees the history.
	d := tinyDataset([]string{"a", "b", "v", "x"}, 4, 2, "v", map[[2]string][]int{
		{"a", "v"}: {5, 5, 0, 0},
		{"b", "v"}: {5, 5, 0, 0},
		{"a", "x"}: {1, 1, 1, 1},
	})
	if got := (TauJSD{}).Scores(d)["v"]; got != 0 {
		t.Fatalf("tau-jsd(v) = %v, want 0", got)
	}
	if got := (TauJSD{Cumulative: true}).Scores(d)["v"]; got <= 0 {
		t.Fatalf("tau-jsd-cumulative(v) = %v, want > 0", got)
	}
	if got := (SilenceDeviance{}).Scores(d)["v"]; got <= 0 {
		t.Fatalf("silence-deviance(v) = %v, want > 0", got)
	}
}

func TestEvidenceAlarm(t *testing.T) {
	// ln(1/0.05) + ln(100) = 7.6009; the alarm needs dev/2 above it.
	if evidenceAlarm(15.2, 100, 0.05) {
		t.Fatal("dev = 15.2 should not alarm")
	}
	if !evidenceAlarm(15.3, 100, 0.05) {
		t.Fatal("dev = 15.3 should alarm")
	}
}
