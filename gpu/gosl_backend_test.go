package gpu_test

import (
	"math"
	"testing"
	"time"

	"github.com/MatheusGrego/itt-engine/analysis"
	"github.com/MatheusGrego/itt-engine/gpu"
	"github.com/MatheusGrego/itt-engine/graph"
)

// TestBackend_ImplementsInterface verifies GoSLBackend satisfies ComputeBackend.
func TestBackend_ImplementsInterface(t *testing.T) {
	var _ gpu.ComputeBackend = (*gpu.GoSLBackend)(nil)
}

func TestBackend_InitAndDeviceInfo(t *testing.T) {
	backend, err := gpu.NewGoSLBackend()
	if err != nil {
		t.Fatalf("NewGoSLBackend failed: %v", err)
	}
	defer backend.Close()

	if !backend.Available() {
		t.Fatal("backend should be available after init")
	}
	if backend.Name() != "gosl" {
		t.Fatalf("Name: want gosl, got %q", backend.Name())
	}

	info := backend.DeviceInfo()
	if info.Backend == "" {
		t.Fatal("DeviceInfo.Backend should not be empty")
	}
	t.Logf("Device: %s", info)
}

func TestBackend_AnalyzeTensions_Parity(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()
	defer backend.Close()

	ig := buildGraph([][3]interface{}{
		{"A", "B", 0.5},
		{"A", "C", 0.3},
		{"B", "C", 0.7},
		{"C", "A", 0.4},
	})

	// GPU backend (float32)
	gpuTensions, err := backend.AnalyzeTensions(ig)
	if err != nil {
		t.Fatalf("AnalyzeTensions failed: %v", err)
	}

	// CPU reference (float64)
	tc := analysis.NewTensionCalculator(analysis.JSD{})
	cpuTensions := tc.CalculateAll(ig)

	// Parity check — relaxed for float32 backend
	for nodeID, cpuT := range cpuTensions {
		gpuT, ok := gpuTensions[nodeID]
		if !ok {
			t.Fatalf("node %q missing from GPU results", nodeID)
		}
		if math.Abs(cpuT-gpuT) > parityEpsilonF32 {
			t.Errorf("node %q: cpu=%.15f gpu=%.15f diff=%.2e", nodeID, cpuT, gpuT, math.Abs(cpuT-gpuT))
		}
	}
}

func TestBackend_AnalyzeTensions_EmptyGraph(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()
	defer backend.Close()

	ig := graph.NewImmutableEmpty()
	tensions, err := backend.AnalyzeTensions(ig)
	if err != nil {
		t.Fatalf("AnalyzeTensions on empty graph failed: %v", err)
	}
	if len(tensions) != 0 {
		t.Fatalf("expected 0 tensions, got %d", len(tensions))
	}
}

func TestBackend_AnalyzeTensions_LargeGraph(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()
	defer backend.Close()

	ig := buildLargeGraph(500, 8)

	gpuTensions, err := backend.AnalyzeTensions(ig)
	if err != nil {
		t.Fatalf("AnalyzeTensions failed: %v", err)
	}

	tc := analysis.NewTensionCalculator(analysis.JSD{})
	cpuTensions := tc.CalculateAll(ig)

	mismatches := 0
	maxDiff := 0.0
	for nodeID, cpuT := range cpuTensions {
		gpuT := gpuTensions[nodeID]
		diff := math.Abs(cpuT - gpuT)
		if diff > maxDiff {
			maxDiff = diff
		}
		if diff > parityEpsilonF32 {
			mismatches++
			if mismatches <= 5 {
				t.Errorf("node %q: cpu=%.15f gpu=%.15f diff=%.2e",
					nodeID, cpuT, gpuT, diff)
			}
		}
	}
	if mismatches > 0 {
		t.Errorf("total mismatches: %d / %d nodes", mismatches, len(cpuTensions))
	}
	t.Logf("Parity verified: %d nodes, 0 mismatches, max diff=%.2e", len(cpuTensions), maxDiff)
}

func TestBackend_CloseIdempotent(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()

	// Close twice — should not panic
	if err := backend.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if backend.Available() {
		t.Fatal("backend should not be available after Close")
	}
}

func TestBackend_AnalyzeAfterClose(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()
	backend.Close()

	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
	})

	_, err := backend.AnalyzeTensions(ig)
	if err == nil {
		t.Fatal("expected error after Close")
	}
}

func TestBackend_ConcurrentAnalyze(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()
	defer backend.Close()

	ig := buildLargeGraph(100, 5)

	// Run 10 concurrent analyses
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := backend.AnalyzeTensions(ig)
			errs <- err
		}()
	}

	for i := 0; i < 10; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent analysis %d failed: %v", i, err)
		}
	}
}

func TestBackend_FiedlerApprox_NotImplemented(t *testing.T) {
	backend, _ := gpu.NewGoSLBackend()
	defer backend.Close()

	_, err := backend.FiedlerApprox(nil, nil)
	if err == nil {
		t.Fatal("expected error for unimplemented FiedlerApprox")
	}
}

func BenchmarkBackend_AnalyzeTensions_100(b *testing.B) {
	backend, _ := gpu.NewGoSLBackend()
	defer backend.Close()
	ig := buildLargeGraph(100, 10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		backend.AnalyzeTensions(ig)
	}
}

func BenchmarkBackend_AnalyzeTensions_1k(b *testing.B) {
	backend, _ := gpu.NewGoSLBackend()
	defer backend.Close()

	g := graph.New()
	now := time.Now()
	for i := 0; i < 1000; i++ {
		for d := 1; d <= 10; d++ {
			target := (i + d) % 1000
			g.AddEdge(nodeID(i), nodeID(target), 0.5, "test", now)
		}
	}
	ig := graph.NewImmutable(g)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		backend.AnalyzeTensions(ig)
	}
}

func BenchmarkBackend_AnalyzeTensions_5k(b *testing.B) {
	backend, _ := gpu.NewGoSLBackend()
	defer backend.Close()
	ig := buildLargeGraph(5000, 10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		backend.AnalyzeTensions(ig)
	}
}

func BenchmarkBackend_AnalyzeTensions_10k(b *testing.B) {
	backend, _ := gpu.NewGoSLBackend()
	defer backend.Close()
	ig := buildLargeGraph(10000, 10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		backend.AnalyzeTensions(ig)
	}
}
