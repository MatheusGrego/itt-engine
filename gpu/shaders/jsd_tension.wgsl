// JSD Tension Kernel — WGSL compute shader
// Transpiled from gpu/jsd_kernel_f32.go
//
// Computes informational tension (average JSD between original and perturbed
// outgoing weight distributions) for each node in a directed weighted graph.
//
// Input: CSR + CSC sparse matrix representation.
// Output: tensions[i] = average JSD for node i.

// ─── Constants ───────────────────────────────────────────────────────────────

const MAX_NEIGHBORS: u32 = 512u;
const MAX_OUT_DEGREE: u32 = 512u;
const EPSILON: f32 = 1e-7;

// ─── Bindings ────────────────────────────────────────────────────────────────

struct Params {
    num_nodes: u32,
    _pad1: u32,
    _pad2: u32,
    _pad3: u32,
}

@group(0) @binding(0) var<storage, read> params: Params;
@group(0) @binding(1) var<storage, read> csr_row_ptr: array<i32>;
@group(0) @binding(2) var<storage, read> csr_col_idx: array<i32>;
@group(0) @binding(3) var<storage, read> csr_values: array<f32>;
@group(0) @binding(4) var<storage, read> csc_col_ptr: array<i32>;
@group(0) @binding(5) var<storage, read> csc_row_idx: array<i32>;
@group(0) @binding(6) var<storage, read_write> tensions: array<f32>;

// ─── Helper: linear search for deduplication ─────────────────────────────────

fn contains_i32(buf: ptr<function, array<i32, MAX_NEIGHBORS>>, count: i32, val: i32) -> bool {
    for (var i: i32 = 0; i < count; i = i + 1) {
        if ((*buf)[i] == val) {
            return true;
        }
    }
    return false;
}

// ─── Collect deduplicated neighbors (out ∪ in) ───────────────────────────────

fn collect_neighbors(
    node_idx: i32,
    buf: ptr<function, array<i32, MAX_NEIGHBORS>>,
) -> i32 {
    var count: i32 = 0;
    let max_buf: i32 = i32(MAX_NEIGHBORS);

    // Outgoing neighbors (CSR row)
    let out_start = csr_row_ptr[node_idx];
    let out_end = csr_row_ptr[node_idx + 1];
    for (var i: i32 = out_start; i < out_end; i = i + 1) {
        let tgt = csr_col_idx[i];
        if (!contains_i32(buf, count, tgt) && count < max_buf) {
            (*buf)[count] = tgt;
            count = count + 1;
        }
    }

    // Incoming neighbors (CSC column)
    let in_start = csc_col_ptr[node_idx];
    let in_end = csc_col_ptr[node_idx + 1];
    for (var i: i32 = in_start; i < in_end; i = i + 1) {
        let source = csc_row_idx[i];
        if (!contains_i32(buf, count, source) && count < max_buf) {
            (*buf)[count] = source;
            count = count + 1;
        }
    }

    return count;
}

// ─── Normalize distribution in-place ─────────────────────────────────────────

fn normalize_in_place(dist: ptr<function, array<f32, MAX_OUT_DEGREE>>, n: i32) {
    var total: f32 = 0.0;
    for (var i: i32 = 0; i < n; i = i + 1) {
        total = total + (*dist)[i];
    }

    if (total == 0.0) {
        let uniform = 1.0 / f32(n);
        for (var i: i32 = 0; i < n; i = i + 1) {
            (*dist)[i] = uniform;
        }
        return;
    }

    for (var i: i32 = 0; i < n; i = i + 1) {
        (*dist)[i] = (*dist)[i] / total;
    }
}

// ─── KL Divergence ───────────────────────────────────────────────────────────

fn kl_div(p: ptr<function, array<f32, MAX_OUT_DEGREE>>,
          q: ptr<function, array<f32, MAX_OUT_DEGREE>>,
          n: i32) -> f32 {
    var sum: f32 = 0.0;
    for (var i: i32 = 0; i < n; i = i + 1) {
        let pi = (*p)[i] + EPSILON;
        let qi = (*q)[i] + EPSILON;
        if (pi > EPSILON) {
            sum = sum + pi * log2(pi / qi);
        }
    }
    return sum;
}

// ─── JSD ─────────────────────────────────────────────────────────────────────

fn jsd(p: ptr<function, array<f32, MAX_OUT_DEGREE>>,
       q: ptr<function, array<f32, MAX_OUT_DEGREE>>,
       m: ptr<function, array<f32, MAX_OUT_DEGREE>>,
       n: i32) -> f32 {
    for (var i: i32 = 0; i < n; i = i + 1) {
        (*m)[i] = 0.5 * (*p)[i] + 0.5 * (*q)[i];
    }
    return 0.5 * kl_div(p, m, n) + 0.5 * kl_div(q, m, n);
}

// ─── Main kernel ─────────────────────────────────────────────────────────────

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let node_idx = i32(gid.x);
    if (u32(node_idx) >= params.num_nodes) {
        return;
    }

    // Step 1: Collect all neighbors (deduplicated union of out + in).
    var neighbor_buf: array<i32, MAX_NEIGHBORS>;
    let num_neighbors = collect_neighbors(node_idx, &neighbor_buf);
    if (num_neighbors == 0) {
        tensions[node_idx] = 0.0;
        return;
    }

    // Step 2: For each neighbor, compute JSD(original, perturbed).
    var total_div: f32 = 0.0;
    var count: i32 = 0;

    var original: array<f32, MAX_OUT_DEGREE>;
    var perturbed: array<f32, MAX_OUT_DEGREE>;
    var m_buf: array<f32, MAX_OUT_DEGREE>;

    for (var ni: i32 = 0; ni < num_neighbors; ni = ni + 1) {
        let n_idx = neighbor_buf[ni];
        let out_start = csr_row_ptr[n_idx];
        let out_end = csr_row_ptr[n_idx + 1];
        let out_degree = out_end - out_start;

        if (out_degree == 0 || out_degree > i32(MAX_OUT_DEGREE)) {
            continue;
        }

        // Build original and perturbed distributions.
        for (var i: i32 = 0; i < out_degree; i = i + 1) {
            let w = csr_values[out_start + i];
            let tgt = csr_col_idx[out_start + i];

            original[i] = w;
            if (tgt == node_idx) {
                perturbed[i] = 0.0;
            } else {
                perturbed[i] = w;
            }
        }

        // Normalize in-place.
        normalize_in_place(&original, out_degree);
        normalize_in_place(&perturbed, out_degree);

        // JSD.
        let div = jsd(&original, &perturbed, &m_buf, out_degree);
        total_div = total_div + div;
        count = count + 1;
    }

    if (count == 0) {
        tensions[node_idx] = 0.0;
    } else {
        tensions[node_idx] = total_div / f32(count);
    }
}
