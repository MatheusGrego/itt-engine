package itt_test

import (
	"fmt"
	"sort"

	itt "github.com/MatheusGrego/itt-engine"
)

// ExampleEngine_readme reproduces the Quick Start example from the README.
func ExampleEngine_readme() {
	engine, _ := itt.NewBuilder().
		Threshold(0.3).
		Build()

	// alice and bob spread their calls over three services. carol only calls
	// service:legacy and dave sends 3 of his 4 calls there.
	events := []itt.Event{
		{Source: "user:alice", Target: "service:api", Weight: 1},
		{Source: "user:alice", Target: "service:db", Weight: 1},
		{Source: "user:alice", Target: "service:cache", Weight: 1},
		{Source: "user:bob", Target: "service:api", Weight: 1},
		{Source: "user:bob", Target: "service:db", Weight: 1},
		{Source: "user:bob", Target: "service:cache", Weight: 1},
		{Source: "user:carol", Target: "service:legacy", Weight: 1},
		{Source: "user:dave", Target: "service:legacy", Weight: 3},
		{Source: "user:dave", Target: "service:api", Weight: 1},
	}
	for _, ev := range events {
		engine.AddEvent(ev)
	}

	// Stop drains the event queue before returning.
	engine.Stop()

	results, _ := engine.Analyze()
	sort.Slice(results.Tensions, func(i, j int) bool {
		return results.Tensions[i].NodeID < results.Tensions[j].NodeID
	})
	for _, r := range results.Tensions {
		fmt.Printf("%-14s tension=%.4f anomaly=%v\n", r.NodeID, r.Tension, r.Anomaly)
	}
	fmt.Printf("Analyzed %d nodes, found %d anomalies\n",
		results.Stats.NodesAnalyzed, results.Stats.AnomalyCount)
	// Output:
	// service:api    tension=0.1732 anomaly=false
	// service:cache  tension=0.1909 anomaly=false
	// service:db     tension=0.1909 anomaly=false
	// service:legacy tension=0.7744 anomaly=true
	// user:alice     tension=0.0000 anomaly=false
	// user:bob       tension=0.0000 anomaly=false
	// user:carol     tension=0.0000 anomaly=false
	// user:dave      tension=0.0000 anomaly=false
	// Analyzed 8 nodes, found 1 anomalies
}
