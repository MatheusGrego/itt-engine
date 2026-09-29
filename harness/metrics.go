package harness

import (
	"math"
	"time"
)

// Options configures a harness run.
type Options struct {
	Config   Config
	Replicas int     // replicas per scenario; seeds 1..Replicas
	Alpha    float64 // significance level for the evidence alarm rule
	TopK     int     // k for hit@k
}

// DefaultOptions returns the plan defaults: 20 replicas, α = 0.05, top 10.
func DefaultOptions() Options {
	return Options{Config: DefaultConfig(), Replicas: 20, Alpha: 0.05, TopK: 10}
}

// Result aggregates one (scenario, method) pair over all replicas.
type Result struct {
	Scenario Scenario
	Method   string
	Replicas int

	// Ranking of the target; NaN in S0, which has no target.
	AUCMean float64
	AUCSD   float64
	HitAtK  float64

	// Alarms under the method's own rule (NaN if it has none).
	AlarmRate float64 // mean fraction of nodes in alarm
	FWER      float64 // fraction of replicas with at least one alarm
	AlarmRule string

	MillisPerReplica float64 // mean time of Scores
}

// Run evaluates every method on every scenario.
func Run(methods []Method, opts Options) []Result {
	var results []Result
	for _, s := range Scenarios {
		agg := make([]struct {
			aucs      []float64
			hits      float64
			alarmRate float64
			anyAlarm  int
			elapsed   time.Duration
		}, len(methods))

		for r := 1; r <= opts.Replicas; r++ {
			d := Generate(s, opts.Config, uint64(r))
			for i, m := range methods {
				start := time.Now()
				scores := m.Scores(d)
				agg[i].elapsed += time.Since(start)

				if d.Target != "" {
					agg[i].aucs = append(agg[i].aucs, AUC(scores, d.Nodes, d.Target))
					agg[i].hits += HitAtK(scores, d.Nodes, d.Target, opts.TopK)
				}
				if a, ok := m.(Alarmer); ok {
					alarms := 0
					for _, id := range d.Nodes {
						if a.Alarm(scores[id], d.N, opts.Alpha) {
							alarms++
						}
					}
					agg[i].alarmRate += float64(alarms) / float64(d.N)
					if alarms > 0 {
						agg[i].anyAlarm++
					}
				}
			}
		}

		for i, m := range methods {
			res := Result{
				Scenario:         s,
				Method:           m.Name(),
				Replicas:         opts.Replicas,
				AUCMean:          math.NaN(),
				AUCSD:            math.NaN(),
				HitAtK:           math.NaN(),
				AlarmRate:        math.NaN(),
				FWER:             math.NaN(),
				MillisPerReplica: float64(agg[i].elapsed.Microseconds()) / 1000 / float64(opts.Replicas),
			}
			if len(agg[i].aucs) > 0 {
				res.AUCMean, res.AUCSD = meanSD(agg[i].aucs)
				res.HitAtK = agg[i].hits / float64(opts.Replicas)
			}
			if a, ok := m.(Alarmer); ok {
				res.AlarmRate = agg[i].alarmRate / float64(opts.Replicas)
				res.FWER = float64(agg[i].anyAlarm) / float64(opts.Replicas)
				res.AlarmRule = a.AlarmRule()
			}
			results = append(results, res)
		}
	}
	return results
}

// AUC returns the fraction of non-target nodes that score below the target,
// with ties counting 0.5.
func AUC(scores map[string]float64, nodes []string, target string) float64 {
	ts := scores[target]
	below, total := 0.0, 0
	for _, id := range nodes {
		if id == target {
			continue
		}
		total++
		switch s := scores[id]; {
		case s < ts:
			below++
		case s == ts:
			below += 0.5
		}
	}
	if total == 0 {
		return math.NaN()
	}
	return below / float64(total)
}

// HitAtK returns the probability that the target ranks in the top k when ties
// are broken uniformly at random. With g nodes scoring above the target and t
// tied with it, the target's rank is g + 1 + U with U uniform on {0..t}.
func HitAtK(scores map[string]float64, nodes []string, target string, k int) float64 {
	ts := scores[target]
	greater, ties := 0, 0
	for _, id := range nodes {
		if id == target {
			continue
		}
		switch s := scores[id]; {
		case s > ts:
			greater++
		case s == ts:
			ties++
		}
	}
	slots := k - greater
	if slots <= 0 {
		return 0
	}
	return math.Min(1, float64(slots)/float64(ties+1))
}

// meanSD returns the mean and the sample standard deviation.
func meanSD(xs []float64) (mean, sd float64) {
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	if len(xs) < 2 {
		return mean, 0
	}
	for _, x := range xs {
		sd += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(sd / float64(len(xs)-1))
}
