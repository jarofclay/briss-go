# 0005 Five detection tiers behind one interface

Status: accepted 2026-11-04; interface in `internal/detect`, tier T0 in
`internal/detect/legacy`.

**Decision.** T0 legacy (Java port), T1 structural (page objects and text lines), T2
classical CV for scans, T3 local ONNX layout model, T4 Claude; an `auto` policy escalates
by document type and agreement; ink-edge snapping in every tier's post-processing.

**Evidence.** The legacy tier keeps headers on scans (IoU 0.85 to 0.86 on the synthetic
scans) but removes them on a real LaTeX book. The small CDLA PicoDet model reaches 73 to
83% acceptance; DocLayout-YOLO (DocStructBench, Apache-2.0 weights) reaches 100% on the
synthetic scans and 88% on the synthetic book with at most one clipped page per 40, at
about 1 s per page on CPU (PLAN.md section 8.6 has the gate status). PP-DocLayout V2 and V3 (Apache-2.0 weights, RapidAI ONNX conversions)
reach 39 and 40 of 40 on the synthetic scans at 0.65 to 0.95 s per page. All three local
models pass on accuracy and miss the 500 ms speed gate on a 4-thread CPU. Claude
choice-only reaches IoU 0.967 with no clipping (0006). The legacy baseline in `briss
bench` on the synthetic corpus is 43.7% accepted with nothing clipped.
