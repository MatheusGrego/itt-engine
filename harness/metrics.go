package harness

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Options configures a harness run.
type Options struct {
	Config   Config
	Replicas int     // replicas per scenario; seeds 1..Replicas
	Alpha    float64 // significance level for the evidence alarm rule
	TopK     int     // k for hit@k

	// Scenarios to run; nil runs every scenario in Scenarios.
	Scenarios []Scenario
	// Workers scores replicas in parallel; ≤ 1 is sequential, which is the
	// only mode where MillisPerReplica is comparable between methods.
	Workers int
	// Generate builds a replica; nil uses Generate.
	Generate func(s Scenario, cfg Config, seed uint64) Dataset
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
	FWERCount int     // replicas with at least one alarm
	Power     float64 // fraction of replicas where the target alarms; NaN in S0
	AlarmRule string

	MillisPerReplica float64 // mean time of Scores
}

// FWERInterval returns the 95% Wilson interval of the FWER.
func (r Result) FWERInterval() (lo, hi float64) { return Wilson(r.FWERCount, r.Replicas) }

// replicaStat is what one method yields on one replica.
type replicaStat struct {
	auc, hit    float64
	hasTarget   bool
	alarmFrac   float64
	anyAlarm    bool
	targetAlarm bool
	elapsed     time.Duration
}

// Run evaluates every method on every scenario of opts.
func Run(methods []Method, opts Options) []Result {
	scenarios := opts.Scenarios
	if scenarios == nil {
		scenarios = Scenarios
	}
	gen := opts.Generate
	if gen == nil {
		gen = Generate
	}
	var results []Result
	for _, s := range scenarios {
		stats := make([][]replicaStat, opts.Replicas)
		parallelFor(opts.Replicas, opts.Workers, func(r int) {
			d := gen(s, opts.Config, uint64(r+1))
			stats[r] = scoreReplica(methods, d, opts)
		})
		results = append(results, aggregate(methods, s, stats, opts)...)
	}
	return results
}

func scoreReplica(methods []Method, d Dataset, opts Options) []replicaStat {
	row := make([]replicaStat, len(methods))
	for i, m := range methods {
		st := &row[i]
		start := time.Now()
		scores := m.Scores(d)
		st.elapsed = time.Since(start)
		if d.Target != "" {
			st.hasTarget = true
			st.auc = AUC(scores, d.Nodes, d.Target)
			st.hit = HitAtK(scores, d.Nodes, d.Target, opts.TopK)
		}
		if a, ok := m.(Alarmer); ok {
			alarms := 0
			for _, id := range d.Nodes {
				if a.Alarm(scores[id], d.N, opts.Alpha) {
					alarms++
					if id == d.Target {
						st.targetAlarm = true
					}
				}
			}
			st.alarmFrac = float64(alarms) / float64(d.N)
			st.anyAlarm = alarms > 0
		}
	}
	return row
}

func aggregate(methods []Method, s Scenario, stats [][]replicaStat, opts Options) []Result {
	results := make([]Result, len(methods))
	for i, m := range methods {
		var aucs []float64
		var hits, alarmRate, power float64
		var anyAlarm int
		var elapsed time.Duration
		for _, row := range stats {
			st := row[i]
			elapsed += st.elapsed
			if st.hasTarget {
				aucs = append(aucs, st.auc)
				hits += st.hit
				if st.targetAlarm {
					power++
				}
			}
			alarmRate += st.alarmFrac
			if st.anyAlarm {
				anyAlarm++
			}
		}
		res := Result{
			Scenario:         s,
			Method:           m.Name(),
			Replicas:         opts.Replicas,
			AUCMean:          math.NaN(),
			AUCSD:            math.NaN(),
			HitAtK:           math.NaN(),
			AlarmRate:        math.NaN(),
			FWER:             math.NaN(),
			Power:            math.NaN(),
			MillisPerReplica: float64(elapsed.Microseconds()) / 1000 / float64(opts.Replicas),
		}
		if len(aucs) > 0 {
			res.AUCMean, res.AUCSD = meanSD(aucs)
			res.HitAtK = hits / float64(len(aucs))
		}
		if a, ok := m.(Alarmer); ok {
			res.AlarmRate = alarmRate / float64(opts.Replicas)
			res.FWERCount = anyAlarm
			res.FWER = float64(anyAlarm) / float64(opts.Replicas)
			res.AlarmRule = a.AlarmRule()
			if len(aucs) > 0 {
				res.Power = power / float64(len(aucs))
			}
		}
		results[i] = res
	}
	return results
}

// parallelFor calls f(0..n-1) on up to workers goroutines.
func parallelFor(n, workers int, f func(i int)) {
	if workers <= 1 {
		for i := 0; i < n; i++ {
			f(i)
		}
		return
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < min(workers, n); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				f(i)
			}
		}()
	}
	wg.Wait()
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

// Wilson returns the 95% Wilson score interval for a proportion of x
// successes in n trials.
func Wilson(x, n int) (lo, hi float64) {
	if n == 0 {
		return math.NaN(), math.NaN()
	}
	const z = 1.959963984540054
	p := float64(x) / float64(n)
	nf := float64(n)
	den := 1 + z*z/nf
	center := (p + z*z/(2*nf)) / den
	half := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / den
	return math.Max(0, center-half), math.Min(1, center+half)
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
