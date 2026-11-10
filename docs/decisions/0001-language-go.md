# 0001 Implement briss-go in Go

Status: accepted 2026-11-04 (PLAN.md section 3).

**Decision.** Go, not Rust.

**Why.** After the rewrite the wall-clock time sits inside PDFium (rasterisation) and,
when enabled, ONNX Runtime; both are the same native code in either language. Go has a
cgo-free single-binary path (PDFium in WebAssembly), the most complete pure PDF writer
(pdfcpu), an official Anthropic SDK, and faster iteration. Spike: the 290-page book is
analysed and cropped in about 0.35 to 0.5 s versus 6 to 9 s for the Java CLI.

**Costs.** Local ML inference needs ONNX Runtime as a separate shared library; Rust could
link tract statically.

**Reopen if** local inference must ship inside one static binary, or the WebAssembly
render budget is missed on target machines. The fallback is the Rust stack in PLAN.md
section 3.4, or a Rust inference module compiled to WASI and run under wazero.
