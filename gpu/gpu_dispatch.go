//go:build cgo

package gpu

import (
	"embed"
	"fmt"
	"sync"
	"unsafe"

	coregpu "cogentcore.org/core/gpu"
)

//go:embed shaders/jsd_tension.wgsl
var shaderFS embed.FS

// gpuPipeline holds the initialized GPU compute system for JSD tension.
type gpuPipeline struct {
	gpu    *coregpu.GPU
	system *coregpu.ComputeSystem
	info   DeviceInfo
}

// Params matches the WGSL Params struct layout (16 bytes, aligned).
type kernelParams struct {
	NumNodes uint32
	_pad1    uint32
	_pad2    uint32
	_pad3    uint32
}

// initGPUPipeline initializes WebGPU and the JSD compute pipeline.
// Returns nil if GPU is not available.
func initGPUPipeline() (*gpuPipeline, error) {
	gp := coregpu.NewComputeGPU()
	if gp == nil {
		return nil, fmt.Errorf("%w: NewComputeGPU returned nil", ErrGPUInitFailed)
	}

	sy := coregpu.NewComputeSystem(gp, "JSDTension")
	vars := sy.Vars()

	// Group 0: all storage buffers
	sgp := vars.AddGroup(coregpu.Storage, "Buffers")

	// binding 0: params (read-only)
	vr := sgp.AddStruct("params", int(unsafe.Sizeof(kernelParams{})), 1, coregpu.ComputeShader)
	vr.ReadOnly = true

	// binding 1: csr_row_ptr (read-only)
	vr = sgp.Add("csr_row_ptr", coregpu.Int32, 1, coregpu.ComputeShader)
	vr.ReadOnly = true

	// binding 2: csr_col_idx (read-only)
	vr = sgp.Add("csr_col_idx", coregpu.Int32, 1, coregpu.ComputeShader)
	vr.ReadOnly = true

	// binding 3: csr_values (read-only)
	vr = sgp.Add("csr_values", coregpu.Float32, 1, coregpu.ComputeShader)
	vr.ReadOnly = true

	// binding 4: csc_col_ptr (read-only)
	vr = sgp.Add("csc_col_ptr", coregpu.Int32, 1, coregpu.ComputeShader)
	vr.ReadOnly = true

	// binding 5: csc_row_idx (read-only)
	vr = sgp.Add("csc_row_idx", coregpu.Int32, 1, coregpu.ComputeShader)
	vr.ReadOnly = true

	// binding 6: tensions (read-write output)
	sgp.Add("tensions", coregpu.Float32, 1, coregpu.ComputeShader)

	sgp.SetNValues(1)

	// Load shader and create pipeline
	pl := coregpu.NewComputePipelineShaderFS(shaderFS, "shaders/jsd_tension.wgsl", sy)
	for i := 0; i < 7; i++ {
		name := []string{"params", "csr_row_ptr", "csr_col_idx", "csr_values", "csc_col_ptr", "csc_row_idx", "tensions"}[i]
		pl.AddVarUsed(0, name)
	}

	sy.Config()

	return &gpuPipeline{
		gpu:    gp,
		system: sy,
		info: DeviceInfo{
			Name:    gp.DeviceName,
			Vendor:  gp.Properties.VendorName,
			Backend: fmt.Sprintf("WebGPU/%s (WGSL)", gp.Properties.BackendType.String()),
		},
	}, nil
}

// dispatch runs the JSD tension kernel on GPU for the given CSR/CSC data.
// Returns float32 tensions indexed by node.
func (p *gpuPipeline) dispatch(
	csrRowPtr []int32, csrColIdx []int32, csrValues []float32,
	cscColPtr []int32, cscRowIdx []int32,
	numNodes int,
) ([]float32, error) {
	sy := p.system
	vars := sy.Vars()

	// Prepare params
	params := []kernelParams{{NumNodes: uint32(numNodes)}}

	// Prepare output buffer
	tensions := make([]float32, numNodes)

	// Upload data to GPU
	if v, err := vars.ValueByIndex(0, "params", 0); err == nil {
		coregpu.SetValueFrom(v, params)
	} else {
		return nil, fmt.Errorf("%w: params: %v", ErrUploadFailed, err)
	}
	if v, err := vars.ValueByIndex(0, "csr_row_ptr", 0); err == nil {
		coregpu.SetValueFrom(v, csrRowPtr)
	} else {
		return nil, fmt.Errorf("%w: csr_row_ptr: %v", ErrUploadFailed, err)
	}
	if v, err := vars.ValueByIndex(0, "csr_col_idx", 0); err == nil {
		coregpu.SetValueFrom(v, csrColIdx)
	} else {
		return nil, fmt.Errorf("%w: csr_col_idx: %v", ErrUploadFailed, err)
	}
	if v, err := vars.ValueByIndex(0, "csr_values", 0); err == nil {
		coregpu.SetValueFrom(v, csrValues)
	} else {
		return nil, fmt.Errorf("%w: csr_values: %v", ErrUploadFailed, err)
	}
	if v, err := vars.ValueByIndex(0, "csc_col_ptr", 0); err == nil {
		coregpu.SetValueFrom(v, cscColPtr)
	} else {
		return nil, fmt.Errorf("%w: csc_col_ptr: %v", ErrUploadFailed, err)
	}
	if v, err := vars.ValueByIndex(0, "csc_row_idx", 0); err == nil {
		coregpu.SetValueFrom(v, cscRowIdx)
	} else {
		return nil, fmt.Errorf("%w: csc_row_idx: %v", ErrUploadFailed, err)
	}
	if v, err := vars.ValueByIndex(0, "tensions", 0); err == nil {
		coregpu.SetValueFrom(v, tensions)
	} else {
		return nil, fmt.Errorf("%w: tensions: %v", ErrUploadFailed, err)
	}

	// Dispatch compute shader
	pl := sy.ComputePipelines["jsd_tension"]
	ce, err := sy.BeginComputePass()
	if err != nil {
		return nil, fmt.Errorf("%w: begin compute pass: %v", ErrComputeFailed, err)
	}
	pl.Dispatch1D(ce, numNodes, 64)
	ce.End()

	// Read back tensions
	tensionVar, err := vars.ValueByIndex(0, "tensions", 0)
	if err != nil {
		return nil, fmt.Errorf("%w: read tensions: %v", ErrDownloadFailed, err)
	}
	tensionVar.GPUToRead(sy.CommandEncoder)
	sy.EndComputePass()
	tensionVar.ReadSync()
	coregpu.ReadToBytes(tensionVar, tensions)

	return tensions, nil
}

// release frees all GPU resources.
func (p *gpuPipeline) release() {
	if p.system != nil {
		p.system.Release()
		p.system = nil
	}
	if p.gpu != nil {
		p.gpu.Release()
		p.gpu = nil
	}
}

// gpuAvailable is a package-level flag for whether GPU dispatch is available.
// Set during NewGoSLBackend based on CGO build tag and GPU initialization.
var (
	gpuPipelineMu     sync.Mutex
	activeGPUPipeline *gpuPipeline
)
