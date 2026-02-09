package itt

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func BenchmarkAddEvent(b *testing.B) {
	e, _ := NewBuilder().ChannelSize(b.N + 1000).Build()
	e.Start(context.Background())
	defer e.Stop()

	ev := Event{Source: "a", Target: "b", Weight: 1.0, Timestamp: time.Now()}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.AddEvent(ev)
	}
	b.StopTimer()

	// Wait for processing
	time.Sleep(200 * time.Millisecond)
}

func BenchmarkSnapshot(b *testing.B) {
	e, _ := NewBuilder().Build()
	e.Start(context.Background())
	defer e.Stop()

	// Seed some data
	for i := 0; i < 100; i++ {
		e.AddEvent(Event{
			Source: fmt.Sprintf("node-%d", i),
			Target: fmt.Sprintf("node-%d", (i+1)%100),
		})
	}
	time.Sleep(200 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap := e.Snapshot()
		snap.Close()
	}
}

func BenchmarkAnalyze_100Nodes(b *testing.B) {
	e, _ := NewBuilder().Build()
	e.Start(context.Background())
	defer e.Stop()

	// Build a graph with 100 nodes
	for i := 0; i < 100; i++ {
		for j := 0; j < 3; j++ { // 3 edges per node
			e.AddEvent(Event{
				Source: fmt.Sprintf("n%d", i),
				Target: fmt.Sprintf("n%d", (i+j+1)%100),
			})
		}
	}
	time.Sleep(500 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap := e.Snapshot()
		snap.Analyze()
		snap.Close()
	}
}

func BenchmarkAnalyze_1kNodes(b *testing.B) {
	e, _ := NewBuilder().ChannelSize(100000).Build()
	e.Start(context.Background())
	defer e.Stop()

	for i := 0; i < 1000; i++ {
		for j := 0; j < 3; j++ {
			e.AddEvent(Event{
				Source: fmt.Sprintf("n%d", i),
				Target: fmt.Sprintf("n%d", (i+j+1)%1000),
			})
		}
	}
	time.Sleep(2 * time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap := e.Snapshot()
		snap.Analyze()
		snap.Close()
	}
}

func BenchmarkAnalyzeNode(b *testing.B) {
	e, _ := NewBuilder().Build()
	e.Start(context.Background())
	defer e.Stop()

	for i := 0; i < 100; i++ {
		e.AddEvent(Event{
			Source: fmt.Sprintf("n%d", i),
			Target: fmt.Sprintf("n%d", (i+1)%100),
		})
	}
	time.Sleep(200 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap.AnalyzeNode("n0")
	}
}

func BenchmarkConcurrentAddEvent(b *testing.B) {
	e, _ := NewBuilder().ChannelSize(b.N + 1000).Build()
	e.Start(context.Background())
	defer e.Stop()

	ev := Event{Source: "a", Target: "b", Weight: 1.0, Timestamp: time.Now()}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			e.AddEvent(ev)
		}
	})
	b.StopTimer()
	time.Sleep(200 * time.Millisecond)
}
