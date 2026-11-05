# 0008 Keep GPL-3.0

Status: accepted 2026-11-04 (PLAN.md section 12).

**Decision.** briss-go stays GPL-3.0-or-later. Clustering, overlay, legacy auto-crop and
split are ports of GPL Java code. Dependencies are Apache-2.0, MIT or BSD (PDFium,
go-pdfium, wazero, pdfcpu, Fyne, ONNX Runtime, anthropic-sdk-go). Model weights must be
Apache or MIT; AGPL inference code (`doclayout_yolo`, `ultralytics`) is never shipped,
exported ONNX graphs run through ONNX Runtime instead.
