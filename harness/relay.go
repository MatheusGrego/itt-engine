package harness

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
)

const streamRelay uint64 = 4

// RelayDataset is a replica of the latent-relay scenario S6 (Tarefa 7).
type RelayDataset struct {
	Dataset
	Relay     string   // the latent node; not in Nodes
	Neighbors []string // the target set: in- and out-neighbours of Relay
	Flows     int      // relayed end-to-end flows
}

// GenerateRelay builds S6 (Tarefa 7). It starts from the S0 graph of seed,
// picks a relay v near the median degree and makes it latent: v and its
// edges are removed from the data, before and after. Each pair (n1, n2)
// with n1 → v and v → n2 in the base graph becomes an end-to-end flow
// n1 → n2 whose events are logged without the hop (the relay is
// transparent), with a lognormal rate of which a fraction rho passes
// through v. From T0 on v is gone (drop true): that fraction is lost and
// not rerouted. With drop false nothing changes (control).
func GenerateRelay(cfg Config, seed uint64, rho float64, drop bool) RelayDataset {
	d := Generate(S0Null, cfg, seed)
	idx := make(map[string]int, len(d.Nodes))
	for i, id := range d.Nodes {
		idx[id] = i
	}
	deg := make([]int, len(d.Nodes))
	for key := range d.Rates {
		deg[idx[key[0]]]++
		deg[idx[key[1]]]++
	}
	order := make([]int, len(d.Nodes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return deg[order[a]] < deg[order[b]] })
	rng := rand.New(rand.NewPCG(seed, streamRelay))
	n := len(d.Nodes)
	lo, hi := n*45/100, n*55/100
	v := d.Nodes[order[lo+rng.IntN(hi-lo)]]

	var in, out []string
	for key := range d.Rates {
		switch v {
		case key[1]:
			in = append(in, key[0])
		case key[0]:
			out = append(out, key[1])
		}
	}
	sort.Strings(in)
	sort.Strings(out)

	// v is latent: drop its edges and the node.
	for key := range d.Rates {
		if key[0] == v || key[1] == v {
			delete(d.Rates, key)
			for t := range d.Counts {
				delete(d.Counts[t], key)
			}
		}
	}
	nodes := make([]string, 0, n-1)
	for _, id := range d.Nodes {
		if id != v {
			nodes = append(nodes, id)
		}
	}
	d.Nodes, d.N, d.Target = nodes, n-1, ""

	flows := 0
	for _, n1 := range in {
		for _, n2 := range out {
			if n1 == n2 {
				continue
			}
			flows++
			key := [2]string{n1, n2}
			rate := lognormal(rng, cfg.RateMu, cfg.RateSigma)
			d.Rates[key] += rate
			for t := 0; t < cfg.T; t++ {
				lambda := rate
				if drop && t >= cfg.T0 {
					lambda *= 1 - rho
				}
				if c := poisson(rng, lambda); c > 0 {
					d.Counts[t][key] += c
				}
			}
		}
	}
	seen := map[string]bool{}
	var nb []string
	for _, id := range append(append([]string{}, in...), out...) {
		if !seen[id] {
			seen[id] = true
			nb = append(nb, id)
		}
	}
	sort.Strings(nb)
	return RelayDataset{Dataset: d, Relay: v, Neighbors: nb, Flows: flows}
}

// AUCSet is the Mann-Whitney AUC of the scores of a target set against the
// rest of nodes: the fraction of (target, other) pairs where the target
// scores higher, ties counting 0.5.
func AUCSet(scores map[string]float64, nodes, targets []string) float64 {
	in := map[string]bool{}
	for _, t := range targets {
		in[t] = true
	}
	var others, tv []float64
	for _, id := range nodes {
		if in[id] {
			tv = append(tv, scores[id])
		} else {
			others = append(others, scores[id])
		}
	}
	if len(tv) == 0 || len(others) == 0 {
		return math.NaN()
	}
	sort.Float64s(others)
	var sum float64
	for _, s := range tv {
		below := sort.SearchFloat64s(others, s)
		eq := sort.SearchFloat64s(others, math.Nextafter(s, math.Inf(1))) - below
		sum += float64(below) + 0.5*float64(eq)
	}
	return sum / float64(len(tv)*len(others))
}

// relayRow aggregates one (rho, drop, method) setting.
type relayRow struct {
	rho                    float64
	drop                   bool
	method                 string
	aucs                   []float64
	nbAlarm, otherAlarm    float64 // mean fraction of neighbours / others in alarm
	anyOther               int     // replicas with ≥ 1 non-neighbour alarm
	neighbours, flows, deg float64
}

// Relay runs Tarefa 7: S6 for rho in {0.2, 0.5, 0.8} and a no-drop control.
func Relay(opts Options) (string, [][]string) {
	methods := []Method{SilenceZ{Cfg: DefaultZConfig()}, SilenceZ{Cfg: CorrectedZConfig()}}
	type setting struct {
		rho  float64
		drop bool
	}
	settings := []setting{{0.2, true}, {0.5, true}, {0.8, true}, {0.5, false}}
	var rows []relayRow
	for _, st := range settings {
		per := make([][]relayRow, opts.Replicas)
		parallelFor(opts.Replicas, opts.Workers, func(r int) {
			rd := GenerateRelay(opts.Config, uint64(r+1), st.rho, st.drop)
			nbSet := map[string]bool{}
			for _, id := range rd.Neighbors {
				nbSet[id] = true
			}
			k := observedDegree(rd.Dataset)
			var degSum float64
			for _, id := range rd.Neighbors {
				degSum += float64(k[id])
			}
			out := make([]relayRow, len(methods))
			for i, m := range methods {
				s := m.Scores(rd.Dataset)
				a := m.(Alarmer)
				var nbA, otA, nOt int
				for _, id := range rd.Nodes {
					al := a.Alarm(s[id], rd.N, opts.Alpha)
					if nbSet[id] {
						if al {
							nbA++
						}
					} else {
						nOt++
						if al {
							otA++
						}
					}
				}
				row := relayRow{aucs: []float64{AUCSet(s, rd.Nodes, rd.Neighbors)},
					nbAlarm:    float64(nbA) / float64(len(rd.Neighbors)),
					otherAlarm: float64(otA) / float64(nOt),
					neighbours: float64(len(rd.Neighbors)), flows: float64(rd.Flows),
					deg: degSum / float64(len(rd.Neighbors))}
				if otA > 0 {
					row.anyOther = 1
				}
				out[i] = row
			}
			per[r] = out
		})
		for i, m := range methods {
			agg := relayRow{rho: st.rho, drop: st.drop, method: m.Name()}
			for _, rep := range per {
				x := rep[i]
				agg.aucs = append(agg.aucs, x.aucs...)
				agg.nbAlarm += x.nbAlarm
				agg.otherAlarm += x.otherAlarm
				agg.anyOther += x.anyOther
				agg.neighbours += x.neighbours
				agg.flows += x.flows
				agg.deg += x.deg
			}
			n := float64(opts.Replicas)
			agg.nbAlarm /= n
			agg.otherAlarm /= n
			agg.neighbours /= n
			agg.flows /= n
			agg.deg /= n
			rows = append(rows, agg)
		}
	}

	csvRows := [][]string{{"rho", "drop", "method", "replicas", "auc_mean", "auc_sd", "neighbour_alarm_frac",
		"other_alarm_frac", "fwer_others", "neighbours", "flows", "neighbour_degree"}}
	var b strings.Builder
	fmt.Fprintf(&b, "Parâmetros: N = %d (menos o relay latente), T = %d, t0 = %d, R = %d réplicas por linha, α = %g. Exploratório.\n\n",
		opts.Config.N, opts.Config.T, opts.Config.T0, opts.Replicas, opts.Alpha)
	b.WriteString("Modelo: o relay v (grau mediano) é latente: v e as arestas dele nunca aparecem nos dados. Cada par (n1 → v, v → n2) do grafo base vira um fluxo ponta a ponta n1 → n2, registrado sem o salto, com taxa lognormal da qual uma fração ρ passa por v. A partir de t0, v some e essa fração é perdida (não é redistribuída). O alvo é o conjunto dos vizinhos de v. No controle, v não some.\n\n")
	b.WriteString("| ρ | v some? | método | AUC vizinhos × resto | vizinhos em alarme | outros em alarme | réplicas com alarme fora dos vizinhos [IC 95%] | nº vizinhos | nº fluxos | grau médio dos vizinhos |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		mean, sd := meanSD(r.aucs)
		lo, hi := Wilson(r.anyOther, opts.Replicas)
		drop := "sim"
		if !r.drop {
			drop = "não (controle)"
		}
		fmt.Fprintf(&b, "| %g | %s | %s | %.3f ± %.3f | %.1f%% | %.3f%% | %.2f [%.2f, %.2f] | %.1f | %.1f | %.1f |\n",
			r.rho, drop, r.method, mean, sd, 100*r.nbAlarm, 100*r.otherAlarm, float64(r.anyOther)/float64(opts.Replicas), lo, hi, r.neighbours, r.flows, r.deg)
		csvRows = append(csvRows, []string{num(r.rho), strconv.FormatBool(r.drop), r.method, strconv.Itoa(opts.Replicas),
			num(mean), num(sd), num(r.nbAlarm), num(r.otherAlarm), num(float64(r.anyOther) / float64(opts.Replicas)),
			num(r.neighbours), num(r.flows), num(r.deg)})
	}
	return b.String(), csvRows
}
