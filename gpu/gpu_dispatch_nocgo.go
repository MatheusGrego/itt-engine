//go:build !cgo

package gpu

// gpuPipeline stub for non-CGO builds (no GPU available).
type gpuPipeline struct{}

// initGPUPipeline always returns nil without CGO (no WebGPU support).
func initGPUPipeline() (*gpuPipeline, error) {
	return nil, nil
}
