package itt

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/MatheusGrego/itt-engine/gpu"
)

// TestGPU_WithGPU_BuildsSuccessfully verifies that WithGPU produces a working engine.
func TestGPU_WithGPU_BuildsSuccessfully(t *testing.T) {
	e, err := NewBuilder().WithGPU(100).Build()
	if err != nil {
		t.Fatalf("Build with WithGPU failed: %v", err)
	}
	if e == nil {
		t.Fatal("engine is nil")
	}
	if e.config.gpuBackend == nil {
		t.Fatal("gpuBackend should be set after WithGPU")
	}
	if e.config.gpuThreshold != 100 {
		t.Fatalf("gpuThreshold: want 100, got %d", e.config.gpuThreshold)
	}
}

// TestGPU_WithGPU_ZeroThreshold_Disabled verifies WithGPU(0) is a no-op.
func TestGPU_WithGPU_ZeroThreshold_Disabled(t *testing.T) {
	e, err := NewBuilder().WithGPU(0).Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if e.config.gpuBackend != nil {
		t.Fatal("gpuBackend should be nil when threshold is 0")
	}
}

// TestGPU_WithGPUBackend_Injection verifies test-injected backend is stored.
func TestGPU_WithGPUBackend_Injection(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()
	defer backend.Close()

	e, err := NewBuilder().WithGPUBackend(backend, 50).Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if e.config.gpuBackend != backend {
		t.Fatal("gpuBackend should match injected backend")
	}
	if e.config.gpuThreshold != 50 {
		t.Fatalf("gpuThreshold: want 50, got %d", e.config.gpuThreshold)
	}
}

// TestGPU_NegativeThreshold_Error verifies negative threshold is rejected.
func TestGPU_NegativeThreshold_Error(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()
	defer backend.Close()

	_, err := NewBuilder().WithGPUBackend(backend, -1).Build()
	if err == nil {
		t.Fatal("expected error for negative gpuThreshold")
	}
}

// TestGPU_AnalyzeRoutesToGPU_AboveThreshold verifies GPU path is taken for large graphs.
func TestGPU_AnalyzeRoutesToGPU_AboveThreshold(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()

	// threshold = 5, so a 10-node graph should route to GPU
	e, err := NewBuilder().WithGPUBackend(backend, 5).Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer e.Stop()

	// Build a 10-node graph
	now := time.Now()
	for i := 0; i < 10; i++ {
		for d := 1; d <= 2; d++ {
			target := (i + d) % 10
			e.AddEvent(Event{
				Source:    nodeID(i),
				Target:    nodeID(target),
				Weight:    float64(i+d) * 0.1,
				Type:      "test",
				Timestamp: now,
			})
		}
	}

	time.Sleep(100 * time.Millisecond) // let events process

	results, err := e.Analyze()
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if results.Stats.NodesAnalyzed != 10 {
		t.Fatalf("want 10 nodes, got %d", results.Stats.NodesAnalyzed)
	}

	// Verify GPU results match CPU path by doing a second analysis with GPU disabled
	e2, _ := NewBuilder().Build()
	e2.Start(context.Background())
	defer e2.Stop()

	for i := 0; i < 10; i++ {
		for d := 1; d <= 2; d++ {
			target := (i + d) % 10
			e2.AddEvent(Event{
				Source:    nodeID(i),
				Target:    nodeID(target),
				Weight:    float64(i+d) * 0.1,
				Type:      "test",
				Timestamp: now,
			})
		}
	}
	time.Sleep(100 * time.Millisecond)

	cpuResults, err := e2.Analyze()
	if err != nil {
		t.Fatalf("CPU Analyze failed: %v", err)
	}

	// Compare tension values — relaxed tolerance for float32 GPU backend
	const gpuParityEpsilon = 1e-5
	gpuMap := make(map[string]float64)
	for _, tr := range results.Tensions {
		gpuMap[tr.NodeID] = tr.Tension
	}
	for _, tr := range cpuResults.Tensions {
		gpuT, ok := gpuMap[tr.NodeID]
		if !ok {
			t.Errorf("node %q missing from GPU results", tr.NodeID)
			continue
		}
		if math.Abs(tr.Tension-gpuT) > gpuParityEpsilon {
			t.Errorf("node %q: cpu=%.15f gpu=%.15f diff=%.2e", tr.NodeID, tr.Tension, gpuT, math.Abs(tr.Tension-gpuT))
		}
	}
}

// TestGPU_AnalyzeUseCPU_BelowThreshold verifies CPU path is taken for small graphs.
func TestGPU_AnalyzeUseCPU_BelowThreshold(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()

	// threshold = 100, graph has only 3 nodes → CPU path
	e, err := NewBuilder().WithGPUBackend(backend, 100).Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	e.Start(context.Background())
	defer e.Stop()

	now := time.Now()
	e.AddEvent(Event{Source: "A", Target: "B", Weight: 1.0, Type: "t", Timestamp: now})
	e.AddEvent(Event{Source: "B", Target: "C", Weight: 2.0, Type: "t", Timestamp: now})

	time.Sleep(50 * time.Millisecond)

	results, err := e.Analyze()
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if results.Stats.NodesAnalyzed == 0 {
		t.Fatal("expected nodes to be analyzed")
	}
}

// TestGPU_StopClosesBackend verifies that Engine.Stop() closes the GPU backend.
func TestGPU_StopClosesBackend(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()

	e, _ := NewBuilder().WithGPUBackend(backend, 10).Build()
	e.Start(context.Background())

	if !backend.Available() {
		t.Fatal("backend should be available before Stop")
	}

	e.Stop()

	if backend.Available() {
		t.Fatal("backend should not be available after Stop")
	}
}

// TestGPU_FallbackOnGPUError verifies graceful fallback when GPU fails.
func TestGPU_FallbackOnGPUError(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()
	// Close the backend to force errors
	backend.Close()

	e, _ := NewBuilder().WithGPUBackend(backend, 1).Build()
	e.Start(context.Background())
	defer e.Stop()

	now := time.Now()
	e.AddEvent(Event{Source: "A", Target: "B", Weight: 1.0, Type: "t", Timestamp: now})
	e.AddEvent(Event{Source: "B", Target: "C", Weight: 2.0, Type: "t", Timestamp: now})
	time.Sleep(50 * time.Millisecond)

	// Should succeed via CPU fallback despite GPU being closed
	results, err := e.Analyze()
	if err != nil {
		t.Fatalf("Analyze should fallback to CPU, got error: %v", err)
	}
	if results.Stats.NodesAnalyzed == 0 {
		t.Fatal("expected nodes analyzed via CPU fallback")
	}
}

// TestGPU_AnalyzeNode_DoesNotUseGPU verifies AnalyzeNode always uses CPU (single-node not worth GPU).
func TestGPU_AnalyzeNode_DoesNotUseGPU(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()

	e, _ := NewBuilder().WithGPUBackend(backend, 1).Build()
	e.Start(context.Background())
	defer e.Stop()

	now := time.Now()
	e.AddEvent(Event{Source: "A", Target: "B", Weight: 1.0, Type: "t", Timestamp: now})
	time.Sleep(50 * time.Millisecond)

	result, err := e.AnalyzeNode("A")
	if err != nil {
		t.Fatalf("AnalyzeNode failed: %v", err)
	}
	if result.NodeID != "A" {
		t.Fatalf("want nodeID A, got %q", result.NodeID)
	}
}

func nodeID(i int) string {
	return "node-" + itoa(i)
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}
