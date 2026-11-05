# Handoff: state of the briss-go rewrite (2026-11-05, after phase 1)

For whoever continues this work. Everything below is committed on branch
`claude/happy-cray-vejte7` of `jarofclay/briss-go`; the branch has not been merged and
no pull request has been opened. Read in this order: this file, `PLAN.md` (the plan,
with "Amended" markers where measurements changed it), `docs/decisions/` (one record per
decision with its evidence), `spike/RESULTS.md` (measurements), `spike/README.md`.

## 1. What exists

| Path | What it is | State |
|---|---|---|
| `cmd/briss` | The binary. Legacy command line `-s`, `-d`, `-p`, `--split-col`, `--split-row` (case-insensitive, like Java), exit codes from PLAN.md section 6 | Working, tested |
| `internal/geom` | Briss ratios, `RectangleHandler` box maths, rotation | Exact vs Java on 64 cases |
| `internal/pdf` | PDFium engine: WebAssembly default with compilation cache, cgo behind `pdfium_cgo`; page sizes, parallel gray rendering, typed password errors, outline/named-dest/link reader | Working, tested |
| `internal/cluster` | Parity plus size-tolerance clustering, Java sampling (float32) | Exact vs Java |
| `internal/overlay` | Streaming overlay with the Java formula and Java target size | Exact size vs Java; profiles within measured tolerance |
| `internal/detect` | `Detector` interface (PLAN.md 4.2) | Done |
| `internal/detect/legacy` | Tier T0, bit-exact port of `CropFinder` and `SplitFinder` | Exact vs Java on Java's overlays |
| `internal/crop` | pdfcpu writer, in-place page cloning, atomic write, keeps encryption | Verified on fixtures, Java outputs, 37 real PDFs |
| `internal/cropdef` | Crop-definition JSON v1 (PLAN.md section 6) | Done |
| `internal/pipeline` | Open, cluster, render into overlays, detect, build crop definitions; golden tests; opt-in real-PDF test | Done |
| `internal/testpdf` | Deterministic fixture PDFs (rotation, crop boxes, outlines, named dests in both styles, links) | Done |
| `testdata/pdfs`, `testdata/golden` | Java sample PDFs plus the synthetic 290-page book; golden JSON from the Java build | Committed (2.1 MB) |
| `legacy-java/` | The Java Briss 2.0, moved unchanged; `tools/golden` regenerates the golden data | Builds with Gradle 7.6 on JDK 11 to 19 |
| `docs/decisions/` | Decision records 0001 to 0009 | Current |
| `spike/` | Week-1 spike programs and tools (separate Go modules) | Kept for reference and for `onnx_eval.py`, `gen_corpus.py`, the GUI and Claude harnesses |
| `.github/workflows` | `go.yml` (vet, test, build) and `gradle.yml` (Java, now in `legacy-java/`) | Both run on `master` pushes and PRs only |

## 2. Decisions taken

See `docs/decisions/README.md`. In one line each: Go (0001); go-pdfium in WebAssembly mode
(0002); pdfcpu with in-place page cloning (0003); Fyne, provisional until a GPU frame
check (0004); five detection tiers behind one interface with ink snapping everywhere
(0005); Claude choice-only with one panel per candidate, default `claude-opus-5-5` (0006);
no credentials in the repository, this is a public fork (0007); GPL-3.0 (0008); the legacy
tier stays bit-compatible and Java quirks are fixed around it (0009).

## 3. Measured facts worth remembering

- Go CLI on the 290-page synthetic book: 0.49 s end to end (Java CLI 6 to 9 s on the same VM). Analysis alone about 0.25 s. 117-page real LaTeX book: 0.9 s.
- WASM pool warm start about 100 ms; first-ever start 4 to 6 s (module compile).
- Golden parity: geometry, clusters, samples, overlay sizes and legacy ratios on Java's overlays are exact. PDFium versus PDFBox rendering moves overlay profiles by 0.66 to 5.23 gray levels and legacy ratios by 0 to 0.33%, except one slide cluster at 2.6% where the legacy "26 of 30" rule flips.
- Java's legacy auto-crop returns a zero-height rectangle on the book's two-page landscape clusters; Go keeps those pages whole.
- Real PDFs (py-pdf/sample-files, 38 files): 37 pass the full pipeline with outlines, named destinations and links intact; 1 has no `/Root` and is skipped.
- DocLayout-YOLO DocStructBench: synthetic scans 40/40 accepted, 0 clipped; book 35/40, 1 clipped; about 1 s per page on 4 CPU threads (misses the 500 ms gate).
- Claude choice-only with panels: Opus 5.5 IoU 0.967, 0 clipped, about 3 c per cluster.
- Fyne editor on a software renderer: 60 fps at 100% scaling (9 of 11 windows), about 40 fps at 4K/200%.

## 4. Gotchas learned the hard way

- Go treats `*_wasm.go` as a GOARCH build constraint: the backends are `engine_webassembly.go` and `engine_cgo.go`.
- go-pdfium v1.21.4 needs Go 1.26 (auto-downloaded). Its `pdfium_experimental` cgo tag needs PDFium headers that match the linked library exactly; `FPDF_GetPageSizeByIndexF` is experimental on cgo, so `internal/pdf` uses `FPDF_GetPageSizeByIndex`.
- A custom wazero `RuntimeConfig` must enable `api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling`.
- Java float32 arithmetic decides real outcomes: the sampled page at step 12 of 145 is index 115 (not 116), and `540 * 4.1666665f` rounds to exactly 2250. Check Java expressions with `jshell` before trusting a hand calculation. The slide deck is 960.12 pt wide, not 960.
- pdfcpu fills `ctx.PageCount` during validation; call `api.ValidateContext` (or `EnsurePageCount`) before using it. `api.ReadContextFile` takes a context first.
- `go vet` rejects unkeyed literals of structs from other packages (`geom.Rect{...}`) in tests.
- Structured outputs reject `minimum`/`maximum` on integer schema fields; thinking tokens count against `max_tokens` on the 5.5 models.
- Several nested candidate outlines on one image made tight candidates look as if they clipped text; one clean panel per candidate fixed the model's choices.
- Fyne's `test.Drag` passes the press position, the GLFW driver the current pointer position plus the delta. Fyne paints on a 60 Hz ticker, so frame intervals bottom out at about 16.7 ms.
- Fyne on Linux needs `libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev wayland-protocols`.
- The community DocLayout-YOLO ONNX export uses Ultralytics metadata (`names`, not `character`) and an end-to-end head; rapid_layout cannot load it, `onnx_eval.py --format yolo-e2e` can. ONNX Runtime dynamic int8 quantization needs `onnx` and `skip_symbolic_shape=True` without `sympy`.
- Background shell loops started with `&` inside a tool call die when the call returns; use the tool's background mode.

## 5. Environment of the cloud sessions

Reachable: PyPI, the Go module proxy, Maven Central (rate-limited; the Google mirror `maven-central.storage-download.googleapis.com` worked), Ubuntu apt, `raw.githubusercontent.com` (so real PDFs from `py-pdf/sample-files` and `mozilla/pdf.js`), `api.anthropic.com`, `huggingface.co`, `www.modelscope.cn`, and since 2026-11-05 `us.aws.cdn.hf.co`. Blocked: GitHub release downloads, `api.github.com`, `cdn-lfs.hf.co`, `cas-bridge.xethub.hf.co`, `cdn-lfs-cn-1.modelscope.cn`, arXiv, archive.org, Gutenberg, Wikimedia. JDK 21 is installed, which Gradle 7.6 cannot run on; compile the Java tree with `javac` against the jars instead (Maven coordinates in `legacy-java/build.gradle`). Xvfb, xdotool and Mesa are installable from apt for GUI checks. Nothing session-specific is needed to continue: venvs, jars, models and caches are disposable and the commands below recreate them.

## 6. Open items, in order

1. **Phase 2, CLI** (PLAN.md section 6): Cobra subcommands `crop`, `detect`, `preview`, `apply`, `bench`, `model`, `config`, `version` on top of `internal/pipeline`; `--save-crops`/`--crops` with `internal/cropdef`; `--exclude` (the page-range parser from `PageNumberParser`); `--json` reports; goreleaser; gitleaks in CI (decision 0007). Keep the legacy flags exactly as they are now.
2. **Real-document labelled set** (PLAN.md 8.5): every detection gate is on synthetic data so far. Label 30 clusters from real files (the 117-page `GeoTopo.pdf`, the multicolumn and rotated samples, plus real scans) and rerun `spike/tools/onnx_eval.py` and `spike/claude`.
3. **Tier T3 speed**: DocLayout-YOLO passes on accuracy but takes about 1 s per page; options are fewer sampled pages per cluster (5), a 640 px export, or a smaller model; measure on a typical laptop. PP-DocLayout_plus-L still needs a `paddle2onnx` export (or allow `cdn-lfs-cn-1.modelscope.cn` for the RapidAI ONNX files).
4. **GPU frame check** (PLAN.md 7.3, decision 0004): one run of `cd spike/gui && BRISS_FRAME_STATS=1 go run . ../testdata/overlays/book300_c00_odd_612x792_p1.png` on a desktop with a GPU, at 100% and 200%.
5. **Phases 3 and 4**: GUI (port `spike/gui/editor.go`), then detection tiers T1, T2, T3, T4 and the `auto` policy (port `spike/claude` into `internal/detect/claude`).
6. Open questions in PLAN.md section 14 ("Jev" is still unidentified).

## 7. Commands

```bash
go test ./...
```

```bash
go build -o briss ./cmd/briss && ./briss -s testdata/pdfs/book300.pdf -d /tmp/book300_cropped.pdf --split-col
```

```bash
BRISS_REAL_PDFS=/path/to/pdfs BRISS_REAL_PDFS_PASSWORDS=openpassword go test -run RealPDFs -v ./internal/pipeline/
```

```bash
(cd legacy-java && ./gradlew shadowJar) && legacy-java/tools/golden/run.sh
```

```bash
python3 -m venv .venv && .venv/bin/pip install reportlab pillow numpy pypdfium2 onnxruntime rapid-layout pypdf onnx
```

```bash
.venv/bin/python spike/tools/gen_corpus.py spike/testdata
```

```bash
.venv/bin/python spike/tools/onnx_eval.py spike/testdata 40 --model doclayout_docstructbench --format yolo-e2e --path doclayout_yolo_docstructbench_imgsz1024.onnx --conf 0.25
```

```bash
cd spike/claude && go build -o claude-spike . && ANTHROPIC_API_KEY="$(cat ~/.config/briss/test.key)" ./claude-spike -mode live -no-adjust -panels -overlays ../testdata/overlays -labels ../testdata/labels.json -out ../testdata/claude-run
```

`spike/testdata/overlays` comes from `cd spike/render && go run . -pdf ../testdata/book300.pdf -out ../testdata/overlays -dump-samples 2`.
Set `BRISS_WAZERO_CACHE=<dir>` to move the compiled-module cache.
