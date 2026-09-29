package harness

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/MatheusGrego/itt-engine/analysis"
	"github.com/MatheusGrego/itt-engine/graph"
)

// Method scores every node of a dataset; a higher score means more anomalous.
type Method interface {
	Name() string
	Scores(d Dataset) map[string]float64
}

// Alarmer is a Method with its own alarm rule, used for the false-positive
// rows under S0.
type Alarmer interface {
	// Alarm reports whether score raises an alarm, given the number of nodes
	// tested and the significance level.
	Alarm(score float64, n int, alpha float64) bool
	// AlarmRule describes the rule for reports.
	AlarmRule() string
}

// TauThreshold is the engine's default static threshold (itt.NewBuilder).
const TauThreshold = 0.2

// DefaultMethods returns M1 to M4 in report order.
func DefaultMethods() []Method {
	return []Method{TauJSD{}, SilenceDeviance{}, BurstDeviance{}, SilenceZ{}, BurstZ{}}
}

// TauJSD is the current engine tension: leave-one-out JSD (with B1) on a graph
// whose weights are the summed counts. M1 sums the after windows [T0, T), so a
// node with no edges after T0 scores 0 (T1). M2 (Cumulative) sums all windows
// [0, T), which is how the engine accumulates weights today.
type TauJSD struct {
	Cumulative bool
}

func (m TauJSD) Name() string {
	if m.Cumulative {
		return "tau-jsd-cumulative"
	}
	return "tau-jsd"
}

func (m TauJSD) Scores(d Dataset) map[string]float64 {
	from := d.T0
	if m.Cumulative {
		from = 0
	}
	g := graph.New()
	for _, id := range d.Nodes {
		g.AddNode(&graph.NodeData{ID: id})
	}
	for key, w := range sumCounts(d, from, d.T) {
		g.AddEdge(key[0], key[1], float64(w), "", time.Time{})
	}
	scores := analysis.NewTensionCalculator(analysis.JSD{}).CalculateAll(g)
	// The graph iterates maps, so sums may differ in the last bits between
	// runs; rounding keeps rankings and ties reproducible.
	for id, s := range scores {
		scores[id] = math.Round(s*1e12) / 1e12
	}
	return scores
}

func (TauJSD) Alarm(score float64, _ int, _ float64) bool { return score > TauThreshold }

func (TauJSD) AlarmRule() string { return fmt.Sprintf("τ > %.1f", TauThreshold) }

// SilenceDeviance is M3 (ADR-0002/0003, v0): the score is S(v), the Poisson
// deviance summed over v's edges that fell below their baseline.
type SilenceDeviance struct{}

func (SilenceDeviance) Name() string { return "silence-deviance-v0" }

func (SilenceDeviance) Scores(d Dataset) map[string]float64 {
	s, _ := Deviance(d)
	return s
}

func (SilenceDeviance) Alarm(score float64, n int, alpha float64) bool {
	return evidenceAlarm(score, n, alpha)
}

func (SilenceDeviance) AlarmRule() string { return "S/2 > ln(1/α) + ln N" }

// BurstDeviance is M4: the score is B(v), the deviance summed over v's edges
// that rose above their baseline. It exists to check S5.
type BurstDeviance struct{}

func (BurstDeviance) Name() string { return "burst-deviance-v0" }

func (BurstDeviance) Scores(d Dataset) map[string]float64 {
	_, b := Deviance(d)
	return b
}

func (BurstDeviance) Alarm(score float64, n int, alpha float64) bool {
	return evidenceAlarm(score, n, alpha)
}

func (BurstDeviance) AlarmRule() string { return "B/2 > ln(1/α) + ln N" }

// evidenceAlarm applies the Bonferroni-style bound from the plan: a deviance
// is twice a log-likelihood ratio in nats, so compare dev/2 with ln(1/α) + ln N.
func evidenceAlarm(dev float64, n int, alpha float64) bool {
	return dev/2 > math.Log(1/alpha)+math.Log(float64(n))
}

// Deviance returns, for every node, the silence S(v) and burst B(v) sums of
// the per-edge Poisson deviance between the after period and the baseline
// from the before period:
//
//	e = mean(counts[0:T0]) * (T - T0), floored at 0.5
//	o = sum(counts[T0:T])
//	d = 2 * (o*ln(o/e) - (o - e)), or 2e when o = 0
//
// An edge counts for both endpoints; it goes into S when o < e and into B
// when o > e. Edges with no events before T0 have e = 0 and an infinite
// deviance for any o > 0; they are left out of both sums (see
// docs/refinement/OPEN-QUESTIONS.md).
func Deviance(d Dataset) (silence, burst map[string]float64) {
	silence = make(map[string]float64, len(d.Nodes))
	burst = make(map[string]float64, len(d.Nodes))
	for _, id := range d.Nodes {
		silence[id] = 0
		burst[id] = 0
	}

	before := sumCounts(d, 0, d.T0)
	after := sumCounts(d, d.T0, d.T)

	// Iterate in a fixed order so the float sums are reproducible.
	keys := make([][2]string, 0, len(before))
	for key := range before {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})

	for _, key := range keys {
		e := float64(before[key]) / float64(d.T0) * float64(d.T-d.T0)
		e = math.Max(e, 0.5)
		o := float64(after[key])
		dev := PoissonDeviance(o, e)
		switch {
		case o < e:
			silence[key[0]] += dev
			silence[key[1]] += dev
		case o > e:
			burst[key[0]] += dev
			burst[key[1]] += dev
		}
	}
	return silence, burst
}

// PoissonDeviance returns 2*(o*ln(o/e) - (o - e)), with the o = 0 limit 2e.
func PoissonDeviance(o, e float64) float64 {
	if o == 0 {
		return 2 * e
	}
	return 2 * (o*math.Log(o/e) - (o - e))
}

// sumCounts sums each edge's counts over windows [from, to); edges with a
// zero sum are left out.
func sumCounts(d Dataset, from, to int) map[[2]string]int {
	sums := make(map[[2]string]int)
	for t := from; t < to; t++ {
		for key, c := range d.Counts[t] {
			sums[key] += c
		}
	}
	return sums
}
