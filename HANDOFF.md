# Handoff: state of the briss-go rewrite (2026-11-10, 2.0 beta)

For whoever continues this work. Everything (the command line, the core, the scan tier
and the desktop app) is on `master` of `jarofclay/briss-go` and tagged `v2.0.0-beta`
(Briss 2.0 beta); the release for the tag is drafted by goreleaser with the command line
archives and the app downloads attached, and the owner publishes it. The working
branches `claude/happy-cray-vejte7` and `claude/gui` were merged into `master`. Read in this order: this file, `PLAN.md` (the plan,
with "Amended" markers where measurements changed it), `docs/decisions/` (one record per
decision with its evidence), `spike/RESULTS.md` (measurements), `spike/README.md`.

## 1. What exists

| Path | What it is | State |
|---|---|---|
| `cmd/briss` | The binary: Cobra subcommands `crop`, `detect`, `apply`, `preview`, `bench`, `version`; the Briss 2.0 flags `-s`, `-d`, `-p`, `--split-col`, `--split-row` still route to the legacy path; exit codes from PLAN.md section 6 | Working, tested |
| `internal/app` | The logic behind the subcommands: analyse and detect, crop, apply with file checks, preview, bench, margins, detector names, error kinds | Done |
| `internal/bench` | Dataset loading (`NAME.pdf` + `NAME.crops.json`), page scoring (IoU, clipped, count, accepted, waste), summaries, Markdown | Done |
| `internal/pagerange`, `internal/preview` | Page lists like `1-3;10`; overlay images with numbered rectangles | Done |
| `.goreleaser.yaml`, `.github/workflows/release.yml` | Release archives for six targets on `v*` tags (draft releases) | Config validated |
| `.gitleaks.toml`, `.pre-commit-config.yaml` | Secret scan with a rule for every Anthropic key kind; CI job and optional local hook | Verified on planted fake keys |
| `docs/datasets.md` | Label format, where labelled data should live, `dir:` predictions, drafting labels from two detectors | Current |
| `spike/tools/layout_propose.py`, `review_sheets.py` | Layout-model predictions as crop definitions; contact sheets with several crop definitions drawn | Working |
| `internal/geom` | Briss ratios, `RectangleHandler` box maths, rotation | Exact vs Java on 64 cases |
| `internal/pdf` | PDFium engine: WebAssembly default with compilation cache, cgo behind `pdfium_cgo`; page sizes, parallel gray rendering, typed password errors, outline/named-dest/link reader | Working, tested |
| `internal/cluster` | Parity plus size-tolerance clustering, Java sampling (float32) | Exact vs Java |
| `internal/overlay` | Streaming overlay with the Java formula and Java target size | Exact size vs Java; profiles within measured tolerance |
| `internal/detect` | `Detector` interface (PLAN.md 4.2) and `PageDetector` for tiers that look at every page | Done |
| `gui/` (branch `claude/gui`) | The desktop app, its own Go module (Fyne needs cgo): dark theme, drop zone, options panel, job list, preview window with editable boxes and result pages; headless tests and a screenshot test | Working; run under Xvfb and the Fyne test driver, not yet on a real desktop |
| `internal/app/process.go` | Outputs (straighten re-fit, normalize), AddOCR | Tested |
| `internal/crop/layer.go` | Content-stream turn about a pivot, invisible Helvetica text layer, also on /Rotate pages | Tested by rendering and text extraction |
| `internal/ocr`, `internal/raster` | Tesseract finder and TSV reader; image turn and crop | Tested (OCR test skips without Tesseract) |
| `.github/workflows/gui.yml` | Runs the core and app tests on Linux, macOS and Windows and builds the downloads (Linux tarball, macOS Briss.app zips for Apple silicon and Intel, Windows Briss.exe) on pushes to `claude/gui` and `master` | Green since run 3 (2026-11-06); the first core test runs on macOS and Windows |
| `internal/detect/cv` | Tier T2 `cv@1` (decision 0010): per-page ink analysis, text lines and figures, two-page split at the gutter; `debug_test.go` (`BRISS_CV_DEBUG="a.png,doc.pdf#3" BRISS_CV_DEBUG_OUT=dir`) draws its decisions, `probe_test.go` lists components and lines in a box | Working, tested; 78.4% of real sheets accepted |
| `internal/pdf/kinds.go` | Page object census (text and image objects, largest image share) used by `auto` to spot scans | Done |
| `internal/detect/legacy` | Tier T0, bit-exact port of `CropFinder` and `SplitFinder` | Exact vs Java on Java's overlays |
| `internal/crop` | pdfcpu writer, in-place page cloning, atomic write, keeps encryption | Verified on fixtures, Java outputs, 37 real PDFs |
| `internal/cropdef` | Crop-definition JSON v1 (PLAN.md section 6) | Done |
| `internal/pipeline` | Open, cluster, render into overlays, detect, build crop definitions; golden tests; opt-in real-PDF test | Done |
| `internal/testpdf` | Deterministic fixture PDFs (rotation, crop boxes, outlines, named dests in both styles, links) | Done |
| `testdata/pdfs`, `testdata/golden` | Java sample PDFs plus the synthetic 290-page book; golden JSON from the Java build | Committed (2.1 MB) |
| `legacy-java/` | The Java Briss 2.0, moved unchanged; `tools/golden` regenerates the golden data | Builds with Gradle 7.6 on JDK 11 to 19 |
| `docs/decisions/` | Decision records 0001 to 0009 | Current |
| `spike/` | Week-1 spike programs and tools (separate Go modules) | Kept for reference and for `onnx_eval.py`, `gen_corpus.py`, the GUI and Claude harnesses |
| `.github/workflows` | `go.yml` (vet, test and build on Linux, macOS and Windows; gitleaks), `release.yml`, `gradle.yml` (Java, in `legacy-java/`) | Run on `master` pushes, PRs to `master` and tags only, so none has run yet on this branch |

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
- Local layout models on the synthetic corpus: DocLayout-YOLO, PP-DocLayoutV2 and PP-DocLayoutV3 all reach 39 to 40 of 40 scans accepted, none or one clipped, and 35 of 40 book pages, at 0.65 to 1.0 s per page (V2 fastest). Accuracy gate passed, speed gate missed.
- Auto-rotate: of the real scans turned four ways (684 cases) 653 are turned back right, 31 (covers, photos) left alone, none wrong; with the cluster-majority rule every sheet of a book stored sideways or upside down is turned right. Stacked-page cut: 0 false cuts on 437 single pages.
- Straighten: on the real scans the detected lean has median 0.5 degrees, 90% under 1.1, at most 3.2; pages that really lean 3 degrees come out level. Projection-profile search over glyph bottoms on the outer 60% of a book page; per-line fits were fooled by ink bridging two lines.
- `crop --straighten --normalize --ocr` on a 16-sheet real scan: 32 pages, two page sizes (left and right), text layer on all, 21 s on 4 threads (OCR is most of it).
- `briss bench` legacy baseline on the synthetic corpus: 43.7% of 467 pages accepted, 0 clipped, mean IoU 0.919.
- Real scans (owner's private set: 10 photocopied books, 171 two-page sheets, draft labels with headers kept): legacy 0% accepted (163 count mismatches), `cv` 78.4% accepted, 12.3% clipped, mean IoU 0.973, 10 s for all sheets; PP-DocLayoutV3 76.0%, 17.5% clipped, about 0.9 s per sheet. `auto` picks `cv` for all ten. Upper bounds until the owner reviews the labels (proposer bias).
- Synthetic corpus with the same build: `cv` 74 of 118 single scans and 24 of 59 two-up sheets; most of its "clips" there are 1.0 to 1.2% because the synthetic labels are layout boxes slightly outside the ink.
- gitleaks' default rules catch only `sk-ant-api03-` keys; `.gitleaks.toml` adds a rule for `sk-ant-<kind>-` keys of any kind.

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
- Fyne: the software painter (test driver, screenshots) panics on rounded rectangles of negative size, so clamp sizes in custom layouts; containers whose children were shown or hidden need a Refresh on the container; `Driver().AllWindows()` has no fixed order, find windows by title; UI changes from goroutines go through `fyne.Do`.
- go-pdfium's WebAssembly build mounts only the working folder's drive on Windows and understands no `C:\` paths, so `internal/pdf` hands PDFium a reader on a file opened in Go instead of a path.
- On arm64 Go fuses `a + b*c` into one rounding; the legacy split needs `a + float32(b*c)` to stay bit-exact with Java.
- pdfium's structured text boxes are in PDF user space (bottom-left origin, unrotated page), not display coordinates.
- `crop.Write` creates the output with `os.CreateTemp` (mode 0600); it now copies the source's permissions before the rename.
- Debug the scan tier on pages rendered by the app's engine (`BRISS_CV_DEBUG="doc.pdf#3"`), not on pypdfium2 renders: the two differ by a grey level or so, and that once moved a gutter decision.
- Judge every scan-tier change with `briss bench` against the labels, not by looking at a few sheets: several fixes that looked right on the sheet in front of me cost more sheets elsewhere (flattening narrow dithered gutters, stray-cluster filters that ate short headings).
- Narrow dark gutters are often dithered and flattening them turns their texture into "ink"; wide binding shadows are smooth and need flattening, or the text running into them is lost. The stacked page edges of a thick book carry fragments of other pages' lines that look like text; requiring lines to be wider than tall and short lines to share a baseline with the text keeps most of them out.
- The draft labels are tight to the ink, so the scan tier's padding moves its score: 0.3% is best on the real set, more padding suits the synthetic labels.

## 5. Environment of the cloud sessions

Reachable: PyPI, the Go module proxy (gitleaks and goreleaser build from it), Maven Central (rate-limited; the Google mirror `maven-central.storage-download.googleapis.com` worked), Ubuntu apt, `raw.githubusercontent.com` (so real PDFs from `py-pdf/sample-files` and `mozilla/pdf.js`), `api.anthropic.com`, `huggingface.co`, `www.modelscope.cn`, and since 2026-11-05 `us.aws.cdn.hf.co` and `cdn-lfs-cn-1.modelscope.cn` (so rapid_layout downloads the PP-DocLayout ONNX files). Blocked: GitHub release downloads, `api.github.com`, `cdn-lfs.hf.co`, `cas-bridge.xethub.hf.co`, arXiv, archive.org, Gutenberg, Wikimedia. JDK 21 is installed, which Gradle 7.6 cannot run on; compile the Java tree with `javac` against the jars instead (Maven coordinates in `legacy-java/build.gradle`). Xvfb, xdotool and Mesa are installable from apt for GUI checks. Nothing session-specific is needed to continue: venvs, jars, models and caches are disposable and the commands below recreate them.

## 6. Open items, in order

1. **Desktop app on real desktops.** (Password prompt, Auto-rotate switch, upright preview and split options added 2026-11-10.) Run the app from the CI downloads (or `cd gui && go build`) on macOS and Windows, check drag and drop, the file chooser, opening outputs, HiDPI, and the frame check of PLAN.md 7.3. Known gaps: macOS "Open with" sends an Apple event Fyne may not pass on; the downloads are unsigned; OCR language is fixed to English in the app (`--ocr-lang` on the command line); the preview edits one sheet at a time; auto-rotate and stacked-page cuts only on scanned pages (born-digital pages with sideways text are left as they are).
2. **Owner review of the draft labels.** The real scans live in the private repository `jarofclay/briss-go.data` (attach it to the session; never copy its files or names into this fork). Draft labels for all 171 sheets are on its branch `claude/draft-labels`, with `LABELING.md` (rules, method, proposer bias) and `labeling/` (both proposals, per-sheet decisions, rebuild script). Once reviewed and merged there, rerun `briss bench --dataset <clone> --detectors legacy,cv,auto` and the layout models through `dir:` predictions, and update the numbers here and in `spike/RESULTS.md`.
3. **Scan-tier follow-ups**: headers cut by the scan border, fragments in stacked page edges beside left pages, pencil and handwritten marks on title pages (the remaining failures in `spike/RESULTS.md`); light grey figure boxes; then the GUI review strip so a user confirms T2 results. More scans (other books, other scanners) are needed before more tuning, to avoid fitting these ten.
4. **Phase 3, GUI parity** (PLAN.md section 7.1, as far as decision 0011 wants it): cluster-wide editing in the preview, Load and Save crops, an OCR language picker.
5. **Phase 4, detection**: tier T1 structural next (T2 is done; T1 decides most born-digital pages), then T3 with the model the real-document bench favours (PP-DocLayoutV2 is fastest; the speed gate needs fewer sampled pages or a smaller input), then T4 by porting `spike/claude` into `internal/detect/claude`, then `auto`, `model pull` and `config` (API key in the OS keychain).
6. **Watch the CLI workflows.** `go.yml` (tests on three systems, gitleaks) first ran on the 2.0 beta pull request; one Linux attempt there failed while downloading modules from the Go proxy, a network error rather than a test failure, so re-run a job that fails that way before looking at the code. `release.yml` first runs on the `v2.0.0-beta` tag; if a later tag fails there, check the goreleaser log before the code.
7. Open questions in PLAN.md section 14 ("Jev" is still unidentified).

## 7. Commands

```bash
go test ./...
```

```bash
go build -o briss ./cmd/briss && ./briss -s testdata/pdfs/book300.pdf -d /tmp/book300_cropped.pdf --split-col
```

```bash
cd gui && go test ./... && go build -o briss-gui .
```

```bash
cd gui && BRISS_GUI_SHOTS=/tmp/shots BRISS_GUI_SAMPLE=../spike/testdata/scan_twoup.pdf go test -run TestScreenshots .
```

```bash
briss bench --dataset /path/to/labels --report bench.md
```

```bash
briss bench --dataset /path/to/briss-go.data --detectors legacy,cv,auto,dir:/path/to/layout-predictions
```

```bash
.venv/bin/python spike/tools/layout_propose.py scan.pdf predictions/scan.crops.json --keep-headers
```

```bash
briss preview scan.pdf --crops scan.crops.json --pages 1-10 --out review/scan
```

```bash
.venv/bin/python spike/tools/review_sheets.py scan.pdf review/ label=scan.crops.json cv=pred/scan.crops.json --cols 4 --rows 3
```

```bash
BRISS_CV_DEBUG="/path/to/scan.pdf#7" BRISS_CV_DEBUG_OUT=/tmp/cvdebug go test -run TestDebugDump -v ./internal/detect/cv/
```

```bash
BRISS_REAL_PDFS=/path/to/pdfs BRISS_REAL_PDFS_PASSWORDS=openpassword go test -run RealPDFs -v ./internal/pipeline/
```

```bash
gitleaks git --no-banner --redact --config .gitleaks.toml .
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
