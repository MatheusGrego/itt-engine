//go:build cgo

package gpu_test

import (
	"testing"

	coregpu "cogentcore.org/core/gpu"
)

// TestGPU_Probe verifies that WebGPU can initialize on this system.
// This is the first real GPU test — if it fails, GoSL won't work.
func TestGPU_Probe(t *testing.T) {
	gp := coregpu.NewComputeGPU()
	if gp == nil {
		t.Fatal("NewComputeGPU returned nil — no WebGPU support")
	}
	defer gp.Release()

	t.Logf("GPU Device: %s", gp.DeviceName)
	t.Logf("GPU Properties: %+v", gp.Properties)
}
