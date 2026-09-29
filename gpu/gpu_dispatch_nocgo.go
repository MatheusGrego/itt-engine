//go:build !cgo

package gpu

import "fmt"

// gpuPipeline stub for non-CGO builds (no GPU available).
// Mirrors the fields and methods used by GoSLBackend so the package
// compiles with CGO_ENABLED=0; initGPUPipeline never returns one.
type gpuPipeline struct {
	info DeviceInfo
}

// initGPUPipeline always returns nil without CGO (no WebGPU support).
func initGPUPipeline() (*gpuPipeline, error) {
	return nil, nil
}

// dispatch is unreachable without CGO; it exists only to satisfy the build.
func (p *gpuPipeline) dispatch(
	csrRowPtr []int32, csrColIdx []int32, csrValues []float32,
	cscColPtr []int32, cscRowIdx []int32,
	numNodes int,
) ([]float32, error) {
	return nil, fmt.Errorf("%w: not available without cgo", ErrGPUNotAvailable)
}

// release is a no-op without CGO.
func (p *gpuPipeline) release() {}
