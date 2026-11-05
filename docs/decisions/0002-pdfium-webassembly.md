# 0002 Read and render with go-pdfium in WebAssembly mode

Status: accepted 2026-11-04; implemented in `internal/pdf`.

**Decision.** go-pdfium's WebAssembly backend (PDFium under wazero) by default; the cgo
backend stays behind the `pdfium_cgo` build tag and is not shipped in 1.0.

**Why.** No cgo and no system libraries; encrypted PDFs, page objects, text geometry,
outlines, named destinations and links are all reachable. Born-digital pages render in
10 to 15 ms and scans in 53 to 124 ms at 900 px; four WASM workers match the native
backend on wall-clock because go-pdfium's cgo backend is single-threaded.

**Costs.** The first start after an install compiles the module for 4 to 6 s; a wazero
compilation cache (default `$XDG_CACHE_HOME/briss-go/wazero`, override with
`BRISS_WAZERO_CACHE`) brings later starts to about 100 ms. Each instance holds its own
linear memory, so the pool is capped at 4 workers, 2 on machines with 8 GB or less, and
one per 150 MB of available memory.

**Gotcha.** A custom wazero runtime config must enable
`api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling`.
