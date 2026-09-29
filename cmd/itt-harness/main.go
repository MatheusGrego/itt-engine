// Command itt-harness runs every tension method on every planted-suppression
// scenario and prints a comparison table (ADR-0006).
//
//	go run ./cmd/itt-harness [-n 500 -t 12 -t0 8 -r 20 -alpha 0.05 -out docs/refinement/results -name results]
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MatheusGrego/itt-engine/harness"
)

func main() {
	opts := harness.DefaultOptions()
	flag.IntVar(&opts.Config.N, "n", opts.Config.N, "number of nodes")
	flag.IntVar(&opts.Config.T, "t", opts.Config.T, "number of time windows")
	flag.IntVar(&opts.Config.T0, "t0", opts.Config.T0, "first window of the after period")
	flag.IntVar(&opts.Replicas, "r", opts.Replicas, "replicas per scenario (seeds 1..r)")
	flag.Float64Var(&opts.Alpha, "alpha", opts.Alpha, "significance level for the evidence alarm rule")
	flag.IntVar(&opts.TopK, "k", opts.TopK, "k for hit@k")
	out := flag.String("out", "docs/refinement/results", "directory for the .csv and .md outputs (empty to skip)")
	name := flag.String("name", "results", "base name of the output files (<name>.csv and <name>.md)")
	flag.Parse()

	if err := validate(opts); err != nil {
		fmt.Fprintln(os.Stderr, "itt-harness:", err)
		os.Exit(2)
	}

	start := time.Now()
	results := harness.Run(harness.DefaultMethods(), opts)
	md := harness.Markdown(results, opts)
	fmt.Print(md)
	fmt.Fprintf(os.Stderr, "itt-harness: %d scenarios x %d replicas in %s\n",
		len(harness.Scenarios), opts.Replicas, time.Since(start).Round(time.Millisecond))

	if *out == "" {
		return
	}
	if err := write(*out, *name, results, md); err != nil {
		fmt.Fprintln(os.Stderr, "itt-harness:", err)
		os.Exit(1)
	}
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

func write(dir, name string, results []harness.Result, md string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var csvBuf bytes.Buffer
	if err := harness.WriteCSV(&csvBuf, results); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, name+".csv"), csvBuf.Bytes(), 0o644); err != nil {
		return err
	}
	doc := "# Resultados do harness\n\nGerado por `go run ./cmd/itt-harness " + strings.Join(os.Args[1:], " ") + "`.\n\n" + md
	return os.WriteFile(filepath.Join(dir, name+".md"), []byte(doc), 0o644)
}
