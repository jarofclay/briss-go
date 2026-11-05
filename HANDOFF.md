# Handoff: state of the briss-go rewrite (2026-11-05)

For whoever continues this work. Everything below is committed on branch
`claude/happy-cray-vejte7` of `jarofclay/briss-go`; the branch has not been merged and
no pull request has been opened. Read in this order: this file, `PLAN.md` (the plan,
with "Amended" markers where the spike changed it), `spike/RESULTS.md` (measured numbers),
`spike/README.md` (how to run each spike).

## 1. What exists

| Path | What it is | State |
|---|---|---|
| `PLAN.md` | Rewrite plan: feature inventory of the Java app, Go-versus-Rust decision, architecture, CLI and GUI design, five-tier detection design, roadmap, licensing, risks, open questions | Current; 31 "Amended" markers from the spike |
| `spike/RESULTS.md` | Week-1 spike memo with all measurements and the gate verdicts | Current |
| `spike/render` | go-pdfium benchmark, WebAssembly (default) and cgo (`-tags pdfium_cgo`) backends, streaming overlay, PNG dumps | Working |
| `spike/cropwrite` | pdfcpu writer: in-place page cloning, box setting, password support | Working, verified |
| `spike/gui` | Fyne crop-editor widget with custom diagonal cursors, `BRISS_FRAME_STATS=1` hook, headless screenshot test | Working headlessly |
| `spike/claude` | Propose-and-choose harness: candidate generation (legacy port plus ink variants), panel images, structured JSON output, scoring; modes dry, mock, live; flags `-no-adjust`, `-panels` | Working, run live |
| `spike/tools/gen_corpus.py` | Synthetic corpus with exact labels (290-page book, 118 single scans, 59 two-up scans) | Working; outputs are gitignored |
| `spike/tools/onnx_eval.py` | Layout-model latency and crop accuracy (`--model`, `--path`, `--exclude`) | Working with the bundled CDLA model |
| `spike/tools/verify_crop.py` | Checks a cropped PDF against its source (text, boxes, outlines, metadata) | Working |
| `src/`, `build.gradle` | The original Java Briss 2.0 fork, untouched | Still the shipping code |

## 2. Decisions taken (do not re-litigate without new data)

1. Language: Go. Rendering, writing and the GUI widget all met their budgets; nothing points at Rust. Rust stays the documented fallback only if a future requirement demands statically linked local ML inference.
2. PDF reading: go-pdfium in WebAssembly mode with a persistent wazero compilation cache. The cgo backend is optional and not a 1.0 deliverable.
3. PDF writing: pdfcpu with in-place page cloning. Outlines, named destinations and links need no renumbering because page objects keep their identity.
4. GUI: Fyne, provisionally. The frame-rate procedure and the no-display fallback are in `PLAN.md` section 7.3.
5. Detection: tiers T0 legacy, T1 structural, T2 CV, T3 local ONNX model, T4 Claude, with ink-edge snapping in the post-processing of every tier. The Claude tier is choice-only (the model picks among locally generated candidates drawn one per panel); free-form coordinate adjustment is dropped because every model got it wrong.
6. Default Claude model `claude-opus-5-5` (only one with zero clips), Sonnet 5.5 as the budget option, user-selectable; `max_tokens` 8192, 30 s timeout with one lower-cap retry.
7. Secrets: never in the repository (public fork). Keys through the environment or the OS keychain only. The test key used on 2026-11-05 was supplied in chat and must be treated as revoked.
8. License stays GPL-3.0; all chosen dependencies and candidate model weights are Apache, MIT or BSD.

## 3. Measured facts worth remembering

- Rendering at the overlay size (900 px tall, gray) instead of 300 DPI RGB is a 13x pixel reduction over the Java pipeline. Book end to end: about 0.35 s in Go versus 6 to 9 s for the Java CLI on the same 4-vCPU VM.
- WASM pool warm start 92 to 130 ms; first-ever start 4 to 6 s (module compile). Peak RSS 126 to 281 MB with 4 workers.
- Native cgo is 2.3 to 2.7x faster per page but single-threaded; 4 WASM workers match it.
- pdfcpu writer: 290 pages to 314 with 24 clones in 33 to 45 ms; output 89% of source size.
- CDLA PicoDet layout model: 53 to 64 ms per page CPU, 73 to 83% acceptance with snapping (fails the 85% gate). The larger Apache-2.0 models are not yet measured.
- Claude, choice-only with panels, 8 synthetic clusters: Opus 5.5 IoU 0.967 and 0 clips at about 3 c per cluster; Sonnet 5.5 0.961 at 1.5 c; Haiku 5.5 0.961 at under 0.1 c.

## 4. Gotchas learned the hard way

- Go treats `*_wasm.go` as a GOARCH build constraint: a file named `pdfium_wasm.go` is silently excluded. The backends are `backend_webassembly.go` and `backend_cgo.go`.
- go-pdfium v1.21.4 needs Go 1.26 (the toolchain auto-downloads it). Its `pdfium_experimental` cgo tag needs PDFium headers that match the linked library exactly.
- A custom wazero `RuntimeConfig` must enable `api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling` or the module fails to compile with "tag section not supported".
- `FPDF_GetPageSizeByIndexF` is experimental on the cgo backend; use `FPDF_GetPageSizeByIndex`.
- Structured outputs reject `minimum`/`maximum` on integer schema fields; put ranges in descriptions and clamp on parse.
- Thinking tokens count against `max_tokens` on the 5.5 models; 1,024 truncated Haiku's answers.
- Drawing several nested candidate rectangles on one image with per-id insets made tight candidates look as if they clipped text; one clean panel per candidate fixed the model's choices.
- Fyne's `test.Drag` passes the press position as the event position, while the GLFW driver passes the current pointer position plus the delta; the widget follows the driver and the test simulates it.
- Fyne on Linux needs `libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev wayland-protocols`.
- The corpus generator originally extended body boxes below the last inked line (trailing blank spacer lines) and originally merged the title and contents pages; both are fixed. Regenerate the corpus after any generator change and rerun every evaluation that reads `labels.json`.
- rapid_layout class names come from the ONNX model metadata and are lowercase for the CDLA model (`header`, `footer`); the evaluation script normalises names.

## 5. Environment of the spike session (cloud container)

Reachable: PyPI, the Go module proxy, Maven Central (rate-limited; a Google mirror worked), Ubuntu apt, `raw.githubusercontent.com`, `api.anthropic.com`, `huggingface.co` and `www.modelscope.cn` pages (after the owner widened the network policy). Blocked: GitHub release downloads, `api.github.com`, the model-hub CDN hosts `us.aws.cdn.hf.co`, `cdn-lfs.hf.co`, `cas-bridge.xethub.hf.co`, `cdn-lfs-cn-1.modelscope.cn`, and arXiv. Workarounds used: `libpdfium.so` from the `pypdfium2` wheel, `libonnxruntime.so` from the `onnxruntime` wheel, PDFium headers from the `chromium/pdfium` GitHub mirror, the CDLA model bundled in the `rapid_layout` wheel. These live in the session scratchpad and are not committed; `spike/README.md` says how to recreate them. The scratchpad also held a Python venv, the compiled Java classes and jars for golden runs, and the wazero cache; all disposable.

## 6. Open items, in order

1. **Real-document corpus.** Build the 30-cluster labelled set from real scanned books and born-digital PDFs (plan section 8.5) and rerun `spike/tools/onnx_eval.py` and `spike/claude` on it. Every gate verdict so far is on synthetic data.
2. **Layout-model evaluation** (plan section 8.7): allow the CDN hosts listed above or run on a laptop, then `python spike/tools/onnx_eval.py spike/testdata 40 --model doclayout_docstructbench` and `--model pp_doc_layoutv2`; export PP-DocLayout_plus-L with `paddle2onnx` and pass `--path`. Decide the T3 gate.
3. **Desktop frame-rate check** (plan section 7.3): `cd spike/gui && BRISS_FRAME_STATS=1 go run . ../testdata/overlays/book300_c00_odd_612x792_p1.png`, drag for ten seconds, pass if p95 under 16 ms. Log the result in `docs/decisions/`.
4. **Phase 1, core library** (plan section 9). Suggested first commits, each small and tested:
   - root `go.mod` (`github.com/jarofclay/briss-go`), `cmd/briss` stub, move the Java tree to `legacy-java/` with its Gradle files, keep its CI job for golden generation;
   - `internal/geom` (briss ratios, PDF user-space conversion, rotation) ported from `RectangleHandler` with property tests;
   - `internal/pdf` from `spike/render` (document, page info, gray render at a target size, page objects and text lines), WASM backend with the compilation cache, cgo behind the build tag;
   - `internal/cluster` and `internal/overlay` from `spike/render` (tolerance clustering, Java sampling rule, streaming overlay with the fallback-to-min rule);
   - `internal/detect/legacy` from `spike/claude/legacy.go` plus the split finder;
   - `internal/crop` from `spike/cropwrite`, plus fixtures with outlines, named destinations and link annotations (generate with pdfcpu's bookmark import and a reportlab file with `linkURL` and named destinations);
   - `internal/cropdef` JSON v1 (plan section 6);
   - golden tests against the Java build on `src/test/resources/pdfs` and the synthetic corpus.
5. **Phase 2, CLI** (plan section 6), then **phase 3, GUI** (plan section 7, port `spike/gui/editor.go`), then **phase 4, detection** (plan section 8).
6. **Plan housekeeping**: create `docs/decisions/` with one file per decision in section 2 above; answer open questions 1, 2, 4, 5, 6, 7, 8 in `PLAN.md` section 14 with the owner ("Jev" is still unidentified).

## 7. Commands to reproduce the spike state from a clean checkout

```bash
python3 -m venv .venv && .venv/bin/pip install reportlab pillow numpy pypdfium2 onnxruntime rapid-layout pypdf
```

```bash
.venv/bin/python spike/tools/gen_corpus.py spike/testdata
```

```bash
cd spike/render && go build -o render . && ./render -pdf ../testdata/book300.pdf -workers 4 -out ../testdata/overlays -dump-samples 2
```

```bash
cd spike/cropwrite && go build -o cropwrite . && ./cropwrite -in ../testdata/book300.pdf -out /tmp/book300_go.pdf -labels ../testdata/labels.json && ../../.venv/bin/python ../tools/verify_crop.py ../testdata/book300.pdf /tmp/book300_go.pdf ../testdata/labels.json ../testdata/cropcheck
```

```bash
cd spike/gui && go test -run TestEditorHeadless -v .
```

```bash
.venv/bin/python spike/tools/onnx_eval.py spike/testdata 40
```

```bash
cd spike/claude && go build -o claude-spike . && ANTHROPIC_API_KEY="$(cat ~/.config/briss/test.key)" ./claude-spike -mode live -no-adjust -panels -overlays ../testdata/overlays -labels ../testdata/labels.json -out ../testdata/claude-run
```

Set `BRISS_WAZERO_CACHE=<dir>` for the render benchmark to measure warm starts. Generated
PDFs, overlays, run outputs and the venv are gitignored.
