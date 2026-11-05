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
about 1 s per page on CPU (PLAN.md section 8.6 has the gate status). Claude choice-only
reaches IoU 0.967 with no clipping (0006).
