// Command itt-harness runs every tension method on every planted-suppression
// scenario and prints a comparison table (ADR-0006).
//
//	go run ./cmd/itt-harness [-n 500 -t 12 -t0 8 -r 20 -alpha 0.05 -out docs/refinement/results -name results]
//
// With -exp it runs one of the fase 2b experiments instead, in parallel on
// every CPU, and writes <name>.md and <name>.csv (the name defaults to
// fase2b-<exp>):
//
//	go run ./cmd/itt-harness -exp discreteness -r 200
package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MatheusGrego/itt-engine/harness"
)

// experiments maps -exp values to their runners and titles. A runner
// returns the markdown body and the CSV rows (header first).
var experiments = map[string]struct {
	title string
	run   func(harness.Options) (string, [][]string)
}{
	"discreteness": {"Tarefa 2: discretude do M5/M6", cond(harness.Discreteness)},
	"stress":       {"Tarefa 3: geradores anti-viés (stress)", cond(harness.Stress)},
	"corrections":  {"Tarefa 4: correções do M5/M6 (q̂ global, φ̂)", cond(harness.Corrections)},
	"scan":         {"Tarefa 6: t0 desconhecido (modo scan)", cond(harness.Scan)},
	"relay":        {"Tarefa 7: relay latente (S6)", harness.Relay},
	"power":        {"Tarefa 5: curva de poder do M5 (limite de detecção empírico)", harness.Power},
}

// cond adapts a runner that returns CondResults.
func cond(f func(harness.Options) (string, []harness.CondResult)) func(harness.Options) (string, [][]string) {
	return func(o harness.Options) (string, [][]string) {
		md, res := f(o)
		return md, harness.CondRows(res)
	}
}

func main() {
	opts := harness.DefaultOptions()
	flag.IntVar(&opts.Config.N, "n", opts.Config.N, "number of nodes")
	flag.IntVar(&opts.Config.T, "t", opts.Config.T, "number of time windows")
	flag.IntVar(&opts.Config.T0, "t0", opts.Config.T0, "first window of the after period")
	flag.IntVar(&opts.Replicas, "r", opts.Replicas, "replicas per scenario (seeds 1..r)")
	flag.Float64Var(&opts.Alpha, "alpha", opts.Alpha, "significance level for the evidence alarm rule")
	flag.IntVar(&opts.TopK, "k", opts.TopK, "k for hit@k")
	flag.Float64Var(&opts.Config.RateMu, "ratemu", opts.Config.RateMu, "mu of the lognormal edge rate (ln 0.5 = low-count regime)")
	flag.Float64Var(&opts.Config.Dispersion, "dispersion", 0, "gamma shape k of gamma-Poisson counts (0 = Poisson)")
	flag.Float64Var(&opts.Config.Trend, "trend", 0, "global rate multiplier (1+trend)^t per window")
	flag.Float64Var(&opts.Config.Seasonality, "season", 0, "amplitude A of the rate multiplier 1 + A sin(2πt/P)")
	flag.IntVar(&opts.Config.SeasonPeriod, "season-period", 4, "period P of the seasonality, in windows")
	flag.Float64Var(&opts.Config.Drift, "drift", 0, "per-edge random walk sd of the log rate, per window (extra)")
	out := flag.String("out", "docs/refinement/results", "directory for the .csv and .md outputs (empty to skip)")
	name := flag.String("name", "", "base name of the output files (default results, or fase2b-<exp> with -exp)")
	exp := flag.String("exp", "", "fase 2b experiment to run instead of the default table: "+expNames())
	flag.Parse()

	if err := validate(opts); err != nil {
		fmt.Fprintln(os.Stderr, "itt-harness:", err)
		os.Exit(2)
	}

	start := time.Now()
	var md string
	var csvBuf bytes.Buffer
	var err error
	if *exp == "" {
		if *name == "" {
			*name = "results"
		}
		results := harness.Run(harness.DefaultMethods(), opts)
		md = harness.Markdown(results, opts)
		err = harness.WriteCSV(&csvBuf, results)
		md = "# Resultados do harness\n\n" + generatedBy() + md
	} else {
		e, ok := experiments[*exp]
		if !ok {
			fmt.Fprintf(os.Stderr, "itt-harness: unknown -exp %q (want one of %s)\n", *exp, expNames())
			os.Exit(2)
		}
		if *name == "" {
			*name = "fase2b-" + *exp
		}
		opts.Workers = harness.ExperimentOptions(opts.Replicas).Workers
		body, rows := e.run(opts)
		w := csv.NewWriter(&csvBuf)
		err = w.WriteAll(rows)
		md = "# " + e.title + "\n\n" + generatedBy() + body
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "itt-harness:", err)
		os.Exit(1)
	}
	fmt.Print(md)
	fmt.Fprintf(os.Stderr, "itt-harness: done in %s\n", time.Since(start).Round(time.Millisecond))

	if *out == "" {
		return
	}
	if err := write(*out, *name, csvBuf.Bytes(), md); err != nil {
		fmt.Fprintln(os.Stderr, "itt-harness:", err)
		os.Exit(1)
	}
}

func generatedBy() string {
	return "Gerado por `go run ./cmd/itt-harness " + strings.Join(os.Args[1:], " ") + "`.\n\n"
}

func expNames() string {
	var names []string
	for k := range experiments {
		names = append(names, k)
	}
	return strings.Join(names, ", ")
}

func validate(opts harness.Options) error {
	cfg := opts.Config
	switch {
	case cfg.N < 20:
		return fmt.Errorf("-n must be at least 20, got %d", cfg.N)
	case cfg.T0 < 1 || cfg.T0 >= cfg.T:
		return fmt.Errorf("need 1 <= t0 < t, got t0=%d t=%d", cfg.T0, cfg.T)
	case opts.Replicas < 1:
		return fmt.Errorf("-r must be positive, got %d", opts.Replicas)
	case opts.Alpha <= 0 || opts.Alpha >= 1:
		return fmt.Errorf("-alpha must be in (0, 1), got %g", opts.Alpha)
	case opts.TopK < 1:
		return fmt.Errorf("-k must be positive, got %d", opts.TopK)
	}
	return nil
}

func write(dir, name string, csvData []byte, md string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, name+".csv"), csvData, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".md"), []byte(md), 0o644)
}
