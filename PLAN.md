# briss-go: Plan for rewriting Briss in Go with AI-assisted crop detection

Status: proposal, 2026-11-04. Scope: replace the Java Briss 2.0 fork in this repository with a
Go application that keeps the same CLI and GUI workflow, is substantially faster, and adds
automatic bounding-box detection so users no longer have to draw every crop rectangle by hand.

---

## 0. Decisions in one page

| Question | Decision | Why (short) |
|---|---|---|
| Language | **Go**, confirmed by a one-week spike with hard exit criteria (section 3.4). Rust is the fallback if the spike fails. | The hot path (PDF rasterization) runs in PDFium in either language. Go gives a cgo-free single binary via PDFium-in-WebAssembly, the most mature pure-Go PDF writer (pdfcpu), an official Anthropic SDK, and faster iteration. Rust wins only on statically linked local ML inference and raw pixel loops, neither of which dominates runtime here. |
| PDF reading / rendering | **go-pdfium** (PDFium) in WebAssembly mode. The cgo backend stays behind a build tag as an optional flavour, not a 1.0 deliverable (Amended) | Same API in both modes. WASM mode needs no system libraries, supports encrypted PDFs, exposes page-object bounds and text geometry for structure-based detection. Spike: 10 to 15 ms per born-digital page and 53 to 124 ms per scanned page at 900 px; four WASM workers match native cgo on wall-clock. |
| PDF writing | **pdfcpu** (pure Go, Apache-2.0) | Box editing, page-tree manipulation, outlines, named destinations, metadata, encryption, all in one dependency with no native code. |
| GUI | **Fyne** with a custom crop-editor widget. Wails is the documented alternative if the 2-day GUI spike shows Fyne cannot reach the needed editor fidelity. | No system webview dependency on Linux, mature packaging, enough control for the rectangle editor. |
| Detection | Four tiers behind one `Detector` interface: legacy port, structural (vector), classical CV for scans, local ML layout model, Claude vision. An `auto` policy escalates by document type and confidence. Every tier runs in the same evaluation harness. | Most born-digital PDFs are solved exactly and instantly by the structural tier. Scans need CV or ML. The VLM tier is reserved for semantic judgment calls and costs about a cent per cluster. |
| Default AI vendor | **Claude** (`claude-opus-5-5` default, user-selectable) for the VLM tier; local models from the Apache-2.0 set (PP-DocLayout_plus-L, DocLayout-YOLO weights, docling heron) for the offline ML tier | Strong at visual judgment with structured JSON output, image and PDF input, official Go SDK, opt-in only. "Jev" from the brief could not be identified (section 8.6); the harness is built so any candidate can be plugged in and measured. |
| License | Keep **GPL-3.0** for briss-go | The clustering, overlay, and auto-crop algorithms are ported from GPL code. All chosen dependencies and model weights are GPL-compatible (section 12). |
| Repository | Go module lives at the repository root; the Java tree moves to `legacy-java/` and stays until the Go build reaches parity, because it generates the golden outputs for compatibility tests | Lets CI compare both implementations on the same PDFs. |

Estimated effort for one engineer: 12 to 16 weeks to a 1.0 with GUI parity and all detection tiers.
With two engineers (one on GUI, one on detection) the critical path is roughly 9 weeks.

**Week-1 spike outcome (2026-11-04, details in `spike/RESULTS.md`):** Go confirmed. Rendering, the PDF writer and the editor widget passed their criteria on a synthetic corpus with exact labels. Fyne is accepted provisionally pending a desktop frame-rate check; section 7.3 gives the procedure and what to do when no display is available. The local layout-model gate is undecided because only a small CDLA model could be downloaded from the spike environment; section 8.7 explains the model-hub access that the evaluation needs. The Claude gate is untested for lack of an API key. The memo's amendments are folded into the sections below and marked "Amended".

---

## 1. Goals, non-goals, constraints

### Goals
1. Feature parity with the Java Briss 2.0 fork in this repository: CLI auto-crop, GUI with cluster overlays and multi-rectangle editing, bookmarks and metadata preserved, password-protected PDFs.
2. Speed: cold start under 200 ms, a 300-page born-digital book clustered and auto-detected in under one second, preview overlays for all clusters in under two seconds on a four-core laptop, output written in under one second. Section 4.5 has the full budget.
3. AI-assisted bounding boxes: the app proposes crop rectangles that are good enough to accept unchanged for the large majority of documents, per cluster and optionally per page, with headers, footers and page numbers removed when the user wants that.
4. Single-binary distribution for Linux, macOS, and Windows, with no JVM and no system PDF library required.
5. Measurable quality: a benchmark harness and labeled dataset so detector changes are judged by numbers, not by eye.

### Non-goals (for 1.0)
- Deskewing or rotating page content by arbitrary angles (PDF page rotation is limited to multiples of 90 degrees; content rotation would require rewriting content streams).
- OCR, text extraction, or any editing beyond page boxes and page duplication.
- A mobile or browser-hosted product. A `briss serve` local web UI is listed as a later option, not a 1.0 deliverable.

### Constraints
- Keep the legacy command line (`-s`, `-d`, `-p`, `--split-col`, `--split-row`) working unchanged.
- Keep the output semantics users rely on: output defaults to `<name>_cropped.pdf`, each crop rectangle becomes its own output page, bookmarks follow the duplicated pages.
- Cloud AI is strictly opt-in. The app must be fully functional offline with no API key.

---

## 2. What Briss does today (inventory from the Java source)

This inventory is the parity checklist. File references are to `src/main/java/at/laborg/briss/`.

### 2.1 Entry points
- `Briss.main`: more than one argument runs the CLI (`cli/BrissCMD`), otherwise the Swing GUI starts, optionally loading a single PDF passed as the only argument.
- CLI flags (`BrissCMD.CommandValues`): `-s source`, `-d dest` (defaults to `<source>_cropped.pdf`), `-p password`, `--split-col`, `--split-row`. Flags are case-insensitive. No usage text, no exit codes beyond `-1` on cluster failure.

### 2.2 Pipeline
1. **Clustering** (`utils/ClusterCreator`, `model/PageCluster`, `model/ClusterDefinition`): for every page take the crop box, falling back to the media box. Cluster key is (even or odd page number, width rounded down to a 20 pt bucket, height rounded down to a 20 pt bucket). Pages the user excluded never merge and become singleton clusters. Clusters are sorted by their lowest page number.
2. **Page sampling**: each cluster renders at most 15 pages (`MAX_MERGE_PAGES`), evenly spaced through the cluster when it has more.
3. **Rendering** (`utils/PDFImageExtractor`, `utils/ClusterRenderWorker`): PDFBox renders every sampled page at **300 DPI RGB** on one background thread, then `ClusterImageData` scales each page to at most 900 px tall and converts to 8-bit gray. This is the dominant cost in the Java app: a Letter page at 300 DPI is 8.4 megapixels, thrown away immediately after a downscale to about 0.6 megapixels.
4. **Overlay image** (`model/ClusterImageData.calculateOverlayOfImages`): per pixel across the sampled pages compute the mean and the standard deviation; the overlay value is `255 - sd`, so content that moves between pages shows up dark and content that is identical on every page (headers, rules) is white. If more than 80% of the non-white pixels are identical across pages, fall back to the per-pixel minimum (darkest) instead, otherwise the overlay would be blank. Implemented with per-pixel `getPixel` calls in nested loops over `int[][]` arrays.
5. **Auto-crop** (`model/CropFinder`, `model/ImageFinderUtil`): project the overlay to one profile per axis (mean of each column and row), take the discrete derivative, compute a sliding standard deviation over 5 samples, then walk inward from each edge until at least 85% of the next 30 samples exceed a threshold of 0.2. The result is a rectangle as four ratios `{left, bottom, right, top}` measured as fractions of the page from each edge (origin bottom-left), the same format used everywhere else.
6. **Split** (`model/SplitFinder`): for `--split-col` or `--split-row`, find the minimum of the same profile within the central 10% of the full overlay (not within the rectangle, noted as a TODO in the code) and divide the rectangle there. Row splits overlap by 1%.
7. **Cropping** (`utils/DocumentCropper`, `utils/RectangleHandler`): two passes with iText 5. Pass one copies every page into a temp file, repeating a page once per rectangle, and re-adds named destinations to the new page numbers. Pass two stamps each output page: pick the smaller of media box and crop box, rotate the ratios by the page `/Rotate` value, scale into that box, and set **both** `/CropBox` and `/MediaBox` to the result. Bookmarks are shifted with `SimpleBookmark.shiftPageNumbers`; the document info dictionary is copied. The destination file is created up front.

### 2.3 GUI (`BrissSwingGUI`, `gui/MergedPanel`, `gui/DrawableCropRect`)
- Start screen with drag-and-drop and a native file dialog; loading prompts for excluded pages (syntax `1-2;34;3-16`, `utils/PageNumberParser`) and for a password if the file is encrypted.
- Progress bar while clustering and rendering.
- One `MergedPanel` per cluster in a wrapping flow layout, showing the overlay image at its native size (max 900 px tall) with a tooltip listing the pages. Each panel starts with the auto-crop rectangle.
- Rectangle editing: draw new, move, resize via eight handles with matching cursors, select with click, multi-select with shift-click, deselect all with Escape, delete via right-click, rectangle numbering and size labels, a ghost rectangle of the last size, copy and paste between panels (`gui/ClipBoard`), split selected rectangle in columns or rows, keyboard nudge and resize, align and size selected rectangles across all panels (`BrissGUIApp`), red highlight when a rectangle is about to be removed, clipping to the image bounds.
- Buttons: Preview (crops to a temp file and opens it in the system viewer), Crop PDF (save dialog, then opens the result in the system viewer).
- Reloading the same file with different excludes keeps existing rectangles (`copyCropsToClusters`).
- Help dialog, donation link, menu with Load and Exit.

### 2.4 Data model to preserve
- Crop rectangle: four floats `{left, bottom, right, top}`, fractions from each page edge, origin bottom-left, relative to the smaller of media box and crop box, applied after rotating by page rotation.
- Cluster: even/odd flag, bucketed width and height, excluded flag, all pages, sampled pages, list of rectangles.
- Crop definition: map page number to list of rectangles (order defines output order).

### 2.5 Known quirks (fix in the rewrite, do not port)
- Bucketing by floor division splits near-identical page sizes that straddle a 20 pt boundary; replace with tolerance-based clustering.
- Split position is computed on the whole overlay rather than inside the rectangle.
- `-d` pointing to a directory is accepted and produces a file named after the directory.
- Rendering at 300 DPI RGB and then downscaling; the overlay loops are per-pixel method calls.
- Export and import of crop definitions exists only as commented-out code (`utils/ExportImportHelper`); the Go version implements it as a JSON format.
- No tests beyond "does not throw" on three sample PDFs (`src/test/java/.../AutoCropTest.java`).

---

## 3. Language evaluation: Go versus Rust

### 3.1 Where the time actually goes
Profile of the Java app on a typical 300-page book, approximate:

| Stage | Java today | Note |
|---|---|---|
| JVM start and Swing init | about 1 s | Gone in either Go or Rust |
| Page metadata and clustering | tens of ms | Trivial in both |
| Rendering 15 pages per cluster at 300 DPI, 2 to 6 clusters | 5 to 20 s | PDFium does this in either language. Rendering directly at the overlay size cuts pixel count roughly 13x |
| Overlay and auto-crop maths | 0.5 to 2 s | Go or Rust loops over byte slices: tens of ms |
| Writing the cropped PDF | 1 to 3 s | pdfcpu or lopdf: well under a second |

The conclusion that drives the decision: after the rewrite, the remaining wall-clock time is inside the PDF rasterizer and, when enabled, inside the ML runtime. Both are native code in either language. The language choice therefore turns on ecosystem fit and distribution, not on raw speed.

### 3.2 Comparison matrix

| Criterion | Go | Rust | Edge |
|---|---|---|---|
| PDF rasterization | go-pdfium (MIT) wraps PDFium (Apache-2.0). WASM mode: no cgo, about 2x slower than native per the project README. cgo mode: native speed, needs a PDFium binary at build time. | pdfium-render wraps the same PDFium (dynamic or static link, native speed). hayro is a pure-Rust rasterizer (Apache-2.0 / MIT) but as of 2025 does not support encrypted PDFs and is not yet performance tuned. | Rust slightly, for native-speed with no cgo. Go matches it with the cgo build tag. |
| PDF structure access (page boxes, rotation, page-object bounds, text geometry) | go-pdfium exposes the full PDFium API including page objects and structured text. pdfcpu reads boxes without PDFium. | pdfium-render exposes the same. lopdf for raw structures. | Tie |
| PDF writing (box edits, page duplication, outlines, named destinations, metadata, encryption) | pdfcpu: high-level API for all of these, pure Go. | lopdf: low level, you write the page-tree and outline fixups yourself. No pure-Rust library at pdfcpu's level of completeness. | Go |
| Local ML inference | onnxruntime_go (cgo) or onnxruntime-purego/gonnx (no cgo, loads the ONNX Runtime shared library at run time). Either way a 15 to 20 MB native library ships beside the binary. No credible pure-Go runtime for YOLO or RT-DETR class models. | ort (links ONNX Runtime, can be static), tract (pure Rust, CPU, statically linked), candle. A single static binary with the model embedded is realistic. | Rust, clearly |
| GUI | Fyne, Gio, Wails. All need cgo on at least one desktop platform; all are workable for an image-plus-rectangles editor. | egui, iced, Slint, Tauri. egui is unusually well suited to an immediate-mode image editor and cross-compiles cleanly. | Slight Rust edge on editor ergonomics, tie on fitness |
| Anthropic API client | Official SDK (`github.com/anthropics/anthropic-sdk-go/v2`) with structured outputs, image and PDF blocks. | No official SDK; raw HTTP via reqwest is fine for the two calls we need. | Go |
| Concurrency model for page-parallel rendering | Goroutines plus a worker pool of PDFium instances; trivial. | Rayon or threads; also trivial, with stronger compile-time guarantees around PDFium handles. | Tie |
| Build, cross-compile, binary size | Fast builds. WASM PDFium adds about 5 MB. cgo GUI toolkits need per-platform builders (CI matrix). | Slower builds. Static binaries easy. Per-platform builders needed for the GUI as well. | Tie |
| Developer velocity and hiring | Simpler language, faster iteration, matches the stated preference of the project. | Steeper curve; stronger correctness guarantees for a long-lived codebase. | Go for this project |
| Raw pixel-loop throughput | Fine; no SIMD without assembly, GC pauses irrelevant at this data size. | Faster (auto-vectorization, no GC). | Rust, but immaterial here (tens of ms) |

### 3.3 Verdict
Go. The measured bottlenecks are inside PDFium and (optionally) ONNX Runtime, which are the same native code in both languages. Go gives a cgo-free default build via PDFium-in-WASM, the most complete PDF writer, the official Claude SDK, and faster iteration, and the user asked for Go.

Rust would be the better choice only if either of these becomes a hard requirement:
- local ML inference must ship inside one static binary with no external runtime (Rust tract or ort static), or
- the Go spike below misses its rendering budget in both WASM and cgo modes.

Hybrid escape hatch, if the Go ML story proves painful: implement the ML detector in Rust (tract), compile it to `wasm32-wasi`, and run it from Go under wazero, exactly as go-pdfium does with PDFium. This keeps the Go app cgo-free while reusing Rust's inference ecosystem. Expect WASM inference to be several times slower than native; it is acceptable for 15 sampled pages per cluster in the background, and the structural and CV tiers do not need it at all.

### 3.4 Confirmation spike (week 1, exit criteria)
Build throwaway programs, no architecture, and measure on the three PDFs in `src/test/resources/pdfs` plus a 300-page scanned book and a 300-page arXiv-style born-digital PDF:

1. go-pdfium WASM: render 15 pages in parallel to gray at 900 px tall. Pass: p50 under 120 ms per page per core, pool startup under 1.5 s, peak RSS under 400 MB.
2. go-pdfium cgo: same test. Record the speedup to decide whether the cgo build is worth maintaining as a release flavor.
3. pdfcpu: duplicate pages by cloning the page dictionary, set both boxes, shift outlines and named destinations, write. Pass: output opens in Acrobat, Preview, and pdf.js; bookmarks land on the right pages; file size within 5% of the Java output for the same crop.
4. Fyne: an image panel with draggable, resizable rectangles and custom cursors at 60 fps on a 4K display. Pass or fail decides Fyne versus Wails.
5. ONNX layout model (DocLayout-YOLO DocStructBench export) through onnxruntime_go on CPU: p50 under 400 ms per 1024 px page.
6. Claude vision on 30 hand-labeled cluster overlays with the propose-and-choose prompt (section 8.4): mean IoU above 0.93 and zero clipped-content cases, latency p50 under 6 s.

Outcome (Amended): 1 passes once the wazero compilation cache is used (warm start 92 to 130 ms; the very first run compiles for 4 to 6 s); 2 measured (2.3 to 2.7x per page, serialised, so no wall-clock win on four cores); 3 passes; 4 passes headlessly, the desktop frame-rate check is open; 5 passes on latency and fails on accuracy for the only model obtainable offline; 6 is blocked on an API key. Rust was not triggered. See `spike/RESULTS.md`.

If 1 and 2 both fail, switch the plan to Rust with pdfium-render, egui, lopdf, and tract; the architecture in section 4 translates one to one.

---

## 4. Target architecture (Go)

### 4.1 Module layout
```
go.mod                         module github.com/jarofclay/briss-go
cmd/briss/                     single binary: CLI subcommands and GUI launcher
internal/pdf/                  Document abstraction over go-pdfium: open (with password), page count,
                               boxes, rotation, render-to-gray at a target size, page objects, text lines
internal/pdf/pdfium_wasm.go    default backend (build tag: none)
internal/pdf/pdfium_cgo.go     native backend (build tag: pdfium_cgo)
internal/cluster/              page clustering (tolerance-based), sampling, cluster ids
internal/overlay/              streaming mean/variance overlay, fallback-to-min rule, PNG export
internal/geom/                 Rect in normalized briss ratios, conversions to PDF user space, rotation
internal/detect/               Detector interface, registry, auto policy, confidence/agreement scoring
internal/detect/legacy/        port of ImageFinderUtil, CropFinder, SplitFinder
internal/detect/structural/    page-object and text-line based detector for born-digital PDFs
internal/detect/cv/            scan-oriented classical detector
internal/detect/onnx/          local layout-model detector (optional build tag or runtime-loaded)
internal/detect/claude/        VLM detector via the Anthropic SDK
internal/crop/                 output writer on pdfcpu: page duplication, box setting, outlines, dests, info
internal/cropdef/              JSON crop-definition format (v1), load/save, validation, hashing
internal/bench/                evaluation harness: datasets, metrics, reports
internal/gui/                  Fyne application: start screen, cluster grid, crop editor widget, dialogs
internal/settings/             config file, API key storage (OS keychain where available), model cache paths
testdata/                      sample PDFs, golden crop definitions produced by the Java build
legacy-java/                   the current Java tree, kept until parity, used by CI for golden generation
```

### 4.2 Core interfaces
```go
// internal/pdf
type Document interface {
    PageCount() int
    Page(i int) PageInfo                         // boxes, rotation, size of the effective box
    RenderGray(i int, maxH int) (*image.Gray, error)   // rendered directly at the overlay size
    PageObjects(i int) ([]ObjectBounds, error)   // text/path/image bounds in user space
    TextLines(i int) ([]TextLine, error)         // line boxes and text, for header/footer detection
    Close() error
}

// internal/detect
type Request struct {
    Doc      pdf.Document
    Cluster  cluster.Cluster      // pages, sampled pages, effective box size
    Overlay  *image.Gray          // may be nil for tiers that do not need it
    Options  Options              // keep headers, margin, split hints, per-page mode
}
type Candidate struct {
    Rects      []geom.Rect        // one or more rectangles in briss ratios
    PerPage    map[int][]geom.Rect // optional per-page overrides
    Confidence float64            // 0..1, tier-defined
    Source     string             // tier name and version, for provenance
    Notes      string
}
type Detector interface {
    Name() string
    Available(ctx context.Context) error       // model missing, no API key, offline
    Detect(ctx context.Context, req Request) (Candidate, error)
}
```
The GUI, the CLI, and the benchmark harness all consume `Candidate` values. Nothing in the app is allowed to talk to PDFium or to a model outside these packages.

### 4.3 Data flow
```
open(path, password)
  -> page metadata (no rendering)            [pdf]
  -> clusters                                [cluster]
  -> for each cluster, in parallel:
       render sampled pages to gray at <=900 px tall   [pdf, worker pool]
       streaming overlay                               [overlay]
       detect(auto policy)                             [detect]
  -> GUI: show overlays + candidate rectangles; user edits
     CLI: accept candidates (or apply crops.json)
  -> crop definition (JSON, also saved for provenance and datasets)   [cropdef]
  -> write output                                         [crop]
```
Structural detection does not depend on the overlay, so for born-digital PDFs the CLI path can skip rendering entirely unless `--preview` is requested. Rendering is still done in the GUI because the user needs to see the overlay.

### 4.4 Concurrency
- A pool of N PDFium instances (each WASM instance holds its own linear memory). Amended after the spike: N = min(GOMAXPROCS, 4) by default, 2 when the machine has 8 GB or less or when sampled pages are image-heavy (a decoded page image above 2 megapixels), and never more than one instance per 150 MB of free memory. Four workers peaked at 281 MB on two-up JPEG scans and 126 MB on a born-digital book; two workers stayed at 200 to 212 MB. Pages are rendered through a channel; results feed the overlay accumulator for their cluster.
- Overlay is computed in streaming form (Welford mean and variance per pixel in `float32` slices), so at most one rendered page per worker is live at a time instead of 15 per cluster.
- Detectors run per cluster in goroutines with a context for cancellation; the GUI updates a cluster panel as soon as its candidate arrives and shows a spinner on the others.
- The Claude tier has a concurrency limit (default 2 in flight) and respects the SDK's retry behavior.

### 4.5 Performance budget (four-core laptop, 300-page Letter PDF)

| Stage | Budget | How |
|---|---|---|
| Cold start to window | < 200 ms warm; the first run after an install or upgrade may spend 4 to 6 s compiling the PDFium module (Amended) | No JVM. The compiled module lives in a wazero compilation cache in the user cache directory (about 20 MB; measured 92 to 130 ms warm start). On the first run the compile happens on a background goroutine while the start screen shows a "preparing PDF engine" notice; the installer or a first-run step may precompile. |
| Open and cluster | < 100 ms | Page boxes from PDFium `GetPageSizeByIndex` or pdfcpu, no page loads |
| Structural detection, all clusters, born-digital | < 500 ms | Page objects for sampled pages only; no raster |
| Overlays for up to 6 clusters x 15 pages at 900 px tall | < 2 s | Direct-size gray render, parallel workers, streaming overlay. Measured (Amended): 0.2 to 0.3 s born-digital, 0.6 to 1.0 s scans, with 4 WASM workers |
| Legacy and CV detection per cluster | < 50 ms | Byte-slice loops, integral images |
| Local ML per cluster (15 pages at 1024 px, CPU) | < 6 s, background | Only when selected or escalated; pages batched |
| Claude per cluster | 2 to 6 s, background | One request per cluster, two to three images |
| Write cropped output | < 1 s | Single pdfcpu pass, page dictionaries cloned in place, streams shared by reference. Measured (Amended): 33 to 45 ms for 290 pages with 24 clones |
| Peak memory, 1000-page book | < 400 MB | Streaming overlay, pool cap, rendered pages released immediately. Measured (Amended): 126 to 281 MB with 4 workers on the spike corpus; the pool cap in section 4.4 keeps image-heavy scans inside the budget |

These budgets become CI benchmarks (`go test -bench` on `testdata/`) with a 20% regression gate.

---

## 5. PDF stack details

### 5.1 Reading and rendering: go-pdfium
- Default build uses the WebAssembly backend (wazero), so `go build` works on any host with no toolchain beyond Go. The embedded module adds about 5 MB.
- The wazero compilation cache must be created with the same core features go-pdfium enables (`api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling`), otherwise the module fails to compile with "tag section not supported". The cache directory sits under the user cache dir and is keyed by the go-pdfium and wazero versions so an upgrade recompiles once (Amended).
- The `pdfium_cgo` build tag switches to the native library. Amended after the spike: per page it is 2.3 to 2.7x faster, but go-pdfium's single-threaded cgo backend serialises calls, so four WASM workers match it on wall-clock on a four-core machine. It is an optional flavour for single-core machines and very large scans, built on demand rather than in the release matrix, and it needs PDFium headers that match the linked library (the `pdfium_experimental` tag failed to build against mismatched headers).
- Encrypted PDFs: open with password; detect `BadPassword` to drive the GUI prompt exactly as the Java app does.
- Rendering: `RenderPageInPixels`-style call at the target height in gray; never render at 300 DPI and downscale.
- Structure: page object bounds (text, path, image, shading, form) and text-page character boxes feed the structural detector. Rotation and effective box are normalized once in `internal/geom`.

### 5.2 Writing: pdfcpu
Amended after the spike (`spike/cropwrite`): the Java two-pass copy-then-stamp approach is not needed. The writer edits the page tree in place:
1. Read and validate the source with pdfcpu (with the password when needed) and resolve every page dictionary before touching the tree, so page numbers stay those of the source.
2. For each page with k rectangles, keep the page dictionary for rectangle 1 and insert k-1 shallow clones right after it in the parent's `/Kids`. Clones share `/Contents`, `/Resources` and `/Annots` by reference, so content is not duplicated. Bump `/Count` on every ancestor.
3. Set `/CropBox` and `/MediaBox` on every output page to the rectangle, computed as in `RectangleHandler.calculateScaledRectangle` (smallest of media and crop box, ratios rotated by `/Rotate`); make `/Rotate` explicit on clones.
4. Nothing to renumber: outlines, named destinations and link annotations reference page objects, and every original page object is still present as the first copy. That is exactly the semantics the Java app produced with `SimpleBookmark.shiftPageNumbers`.
5. Keep the info dictionary and XMP metadata; an encrypted source stays encrypted with the same password.
6. Write with object streams and xref streams enabled for size.

Measured on the spike corpus: 290 pages became 314 with 24 clones, zero text or box mismatches, 26 of 26 outline targets correct, output at 89% of the source size (the Java output was 101 to 113%), 33 to 45 ms end to end. Named destinations were not exercised because the generated corpus had none; phase 1 adds fixtures for them and for link annotations (section 10).

---

## 6. CLI design

Cobra-based, one binary. Legacy flags remain exactly as they are, so existing scripts and wrappers keep working.

```
briss                                  start GUI
briss file.pdf                         start GUI with file loaded
briss -s in.pdf [-d out.pdf] [-p pw] [--split-col] [--split-row]
                                       legacy auto-crop, unchanged semantics (legacy detector)

briss crop in.pdf [-o out.pdf] [options]
    --detector auto|legacy|structural|cv|ml|claude      default auto
    --per-page                 one rectangle per page instead of per cluster (scans with drift)
    --margin 3pt               padding around detected content (pt, px or %)
    --keep-headers             do not remove running headers, footers, page numbers
    --split-col / --split-row  as before, but computed inside the rectangle
    --exclude "1-3;10"         pages to keep out of clustering (singleton clusters, never cropped by others)
    --password pw | --password-stdin
    --crops crops.json         skip detection, apply a saved crop definition
    --save-crops crops.json    write the crop definition that was applied
    --dry-run                  detect and report, write nothing
    --json                     machine-readable report on stdout
    --threads N, --quiet, --verbose

briss detect in.pdf [--detector ...] [--json]     print candidates, no output file
briss preview in.pdf [--out dir] [--cluster N]     write overlay PNGs with rectangles drawn
briss apply in.pdf --crops crops.json -o out.pdf   same as crop --crops
briss bench --dataset dir [--detectors a,b,c] [--report out.md]
briss model pull <name> | list | remove             local ML models (downloaded to the user cache dir)
briss config set claude.api_key ... | get | path    settings and key storage
briss version
```
Exit codes: 0 success, 1 usage, 2 input error (missing file, bad password), 3 detection unavailable (no model, no key), 4 write error. The legacy entry keeps its current behavior of printing a message and returning non-zero.

Crop definition JSON (v1) is the contract between CLI, GUI, and the dataset:
```json
{
  "version": 1,
  "source": {"path": "book.pdf", "sha256": "...", "pages": 312},
  "options": {"keep_headers": false, "margin_pt": 3},
  "clusters": [
    {"id": 0, "pages": [1,3,5], "even": false, "box_pt": [612, 792],
     "rects": [{"l": 0.12, "b": 0.09, "r": 0.11, "t": 0.08}],
     "source": "structural@1.0", "confidence": 0.97}
  ],
  "page_overrides": {"17": [{"l": 0.10, "b": 0.09, "r": 0.11, "t": 0.08}]},
  "excluded_pages": [1, 2]
}
```

---

## 7. GUI design (Fyne)

### 7.1 Parity screens
1. **Start screen**: drop zone and Open button, recent files, version, link to help. Dropping or opening a PDF prompts for a password only if needed, then for excluded pages (same syntax, prefilled from the last run on that file).
2. **Cluster grid**: wrapping grid of cluster panels, each showing the overlay at display scale with a header "Odd pages, 156 pages: 1, 3, 5 ..." and a status chip (detector used, confidence, elapsed).
3. **Crop editor widget** (custom Fyne widget): draw, move, resize with eight handles and matching cursors, click to select, shift-click for multi-select, Escape to deselect, Delete or right-click to remove, numbering and size labels, ghost rectangle of the last drawn size, copy and paste across panels, split in columns or rows inside the rectangle, arrow-key nudge and shift-arrow resize, align and equalize selected rectangles across panels, clipping to the image bounds, red tint when a rectangle would be removed.
4. **Footer**: progress bar, Preview (temp crop opened in the system viewer), Crop PDF (save dialog, then open in viewer).
5. **Menu**: Load, Reload with different excludes (keeps rectangles), Save crops, Load crops, Settings, Help, Exit.

### 7.2 Additions for detection
- **Detect** button per panel and for all panels, with a detector picker (Auto, Structural, Scan, Local model, Claude). Candidates appear as dashed suggestions with Accept and Reject; Accept replaces the panel's rectangles. Keyboard: A to accept, R to reject.
- **Confidence cues**: panels whose tiers disagree (IoU below 0.9 between the two best tiers) get an amber border and are listed first in a "needs review" strip.
- **Page strip**: thumbnails of the sampled pages under the overlay; clicking one shows that page with the rectangle so the user can check for clipping. In per-page mode, each page carries its own rectangle.
- **Settings**: default detector, margin, keep headers, Claude model and API key (stored in the OS keychain where available, else in the config file with a warning), local model download, telemetry off by default, "contribute corrections" opt-in that saves anonymized crop definitions plus overlay PNGs to a local folder the user can share for the dataset.

### 7.3 Why Fyne, and the fallback
Fyne is pure Go apart from the OpenGL window layer, needs no system webview, has an official packager (`fyne package`), and its event model (Draggable, Hoverable, Cursorable, keyboard shortcuts) covers the editor. The risk is editor fidelity (custom cursors for diagonal resize, HiDPI image scaling, redraw cost), which spike item 4 checks. If Fyne fails, Wails v2 with a TypeScript canvas editor is the fallback: better editor ergonomics and a free path to a `briss serve` browser mode, at the price of a WebKitGTK dependency on Linux and WebView2 on Windows.

**Frame-rate check, and what to do without one (Amended).** The spike editor passes its headless test and ships with a measurement hook: with `BRISS_FRAME_STATS=1` it prints the median and 95th percentile of the interval between redraws every 60 redraws while you drag. Procedure, in order of preference:
1. Any desktop, any display: run `go run .` in `spike/gui` (later the phase-3 editor) with `BRISS_FRAME_STATS=1`, drag and resize a rectangle for ten seconds at the display's native resolution and again at 200% scaling. Pass: p95 under 16 ms. A 4K display is not required for a meaningful number. The editor draws a 900 px image and a few dozen rectangles, so GPU cost is dominated by object count, not display pixels; 4K only adds the one-time image upload.
2. No desktop at all: Fyne is provisionally accepted and phase 3 starts with the hook in place. The first build that someone with a screen can run (end of phase-3 week 1) is the decision point; that person runs step 1 and logs the result in `docs/decisions/`.
3. Step 1 fails on a current laptop: try the two cheap fixes first (draw the image through `canvas.Raster` instead of `canvas.Image`, and rebuild only the changed rectangle's canvas objects instead of the whole list). If p95 stays above 16 ms, switch the editor to Wails. The core packages are untouched because the GUI only consumes crop definitions and overlay images.

Worked example of the worst case: nobody on the project has a display until phase 5. Then 1.0 ships the Fyne editor with the hook enabled behind the environment variable, the release notes ask users on 4K displays to report the printed numbers, and the Wails port is a budgeted post-1.0 item triggered by those reports rather than by a measurement the team could not take.

---

## 8. AI-assisted bounding-box detection

### 8.1 Problem statement
Given a cluster (pages with the same size and parity) or a single page, produce one or more rectangles in briss ratio coordinates such that:
- no content the user wants to keep is clipped (hard constraint; a clipped line of body text is a failure, a wasted margin is only a cost);
- running headers, footers, and page numbers are excluded unless `keep_headers` is set;
- marginal noise from scanning (black edges, punch holes, bleed-through, specks) is excluded;
- two-up scans and multi-column layouts can be split into one rectangle per page or column when asked;
- rectangles are consistent across a cluster (same size for all odd pages, for example) unless per-page mode is on, because readers prefer stable page geometry;
- the result carries a confidence score and provenance.

The Java auto-crop is a projection-profile heuristic that cannot distinguish a header from body text, hence the historical reliance on manual rectangles.

### 8.2 Detection tiers

| Tier | Name | Input | Method | Strengths | Weaknesses | Cost / latency |
|---|---|---|---|---|---|---|
| T0 | `legacy` | overlay | Port of `ImageFinderUtil` and `CropFinder` | Bit-compatible with Java; deterministic | Keeps headers; fooled by noise | free, < 10 ms |
| T1 | `structural` | page objects, text lines | Union of object bounds per page; running header/footer detection by cross-page repetition of text-line position and normalized text (same y within 2 pt on more than 60% of the cluster, short text, or digit/roman-numeral pattern at the top or bottom band); optional margin-note detection (narrow text column outside the main column, user choice); per-page boxes then cluster harmonization (union, or 95th percentile when `--per-page` is off and outliers exist) | Exact to the point, no rendering, instant, explainable | Only born-digital PDFs; fails on scans (one image object) and on PDFs with full-page background images or invisible objects (mitigated by ignoring objects outside the media box, white fills, and zero-alpha objects) | free, < 5 ms per page |
| T2 | `cv` | rendered pages (gray) | Sauvola adaptive threshold, border-noise removal (dark connected components touching the edge), despeckle, run-length smoothing to form line and block components, projection profiles with valley detection for two-up and column splits, header/footer by repetition of line positions across sampled pages, text-line height statistics to tell page numbers from body text | Offline, fast, good on clean scans | Struggles with marginalia, figures touching the margin, heavily skewed scans, decorative borders | free, 20 to 60 ms per page |
| T3 | `ml` | rendered pages (RGB, 640 to 1024 px) | ONNX layout detector; crop = union of boxes in keep-classes, excluding header, footer, page-number and (optionally) aside classes; per-page then harmonized. Candidates: PP-DocLayout_plus-L (Apache-2.0, RT-DETR-L, has header, footer, number, aside_text classes), DocLayout-YOLO DocStructBench weights (Apache-2.0 per the model cards; its `abandon` class covers headers, footers and page numbers; the AGPL `doclayout_yolo` runtime is not shipped because inference goes through ONNX Runtime), docling-layout-heron (Apache-2.0, RT-DETRv2, about 172 MB) | Handles scans, mixed layouts, figures, tables; semantic classes map directly onto the crop policy | Needs ONNX Runtime beside the binary (15 to 20 MB) and a model download (40 to 170 MB); CPU latency hundreds of ms per page; boxes are at 640 to 1024 px resolution so edges need snapping | free after download; measured 53 to 64 ms per page for a small PicoDet model, estimated 150 to 800 ms for the RT-DETR candidates, on CPU (Amended) |
| T4 | `claude` | overlay plus two representative pages | Propose-and-choose (section 8.4): the app computes candidate rectangles from T1 to T3 and from simple variants (with and without header band, with and without margin notes, column splits), draws them numbered on the images, and asks Claude to pick and optionally adjust; structured JSON output; result snapped to projection-profile edges | Best at semantic judgment (what is a margin note, which band is a header, is this a two-up scan); explains itself; one call per cluster | Network, API key, cost, privacy (page images leave the machine), nondeterminism, coordinate precision is only moderate when asked to regress raw coordinates (hence propose-and-choose) | about 1 to 2 cents per cluster on Opus 5.5; 2 to 6 s |

`auto` policy:
1. Classify the document: born-digital if sampled pages have text objects covering the content area and no single image object covering more than 90% of the page; otherwise scanned.
2. Born-digital: run T1 and T0. If their IoU is above 0.9 after T0 is adjusted for the header band, accept T1 with high confidence. Otherwise run T3 if installed, then T4 if enabled, else mark for review.
3. Scanned: run T2. Run T3 if installed. Agreement above 0.9 accepts; otherwise T4 if enabled, else mark for review with both candidates shown.
4. Any tier reporting a two-up valley or multi-column signal adds split candidates; `auto` never splits unless the user asked or a two-up scan is detected with high confidence.
5. Post-processing for every tier: apply margin, snap edges to the nearest ink boundary within 1% of page size, harmonize across the cluster, clamp to the page.

### 8.3 Tooling evaluation for the AI tier
Candidates named in the brief were "Jev", Claude, or other. "Jev" could not be identified (no tool, model, or vendor by that name turned up in document-layout or vision-model searches); see the open question in section 14. The evaluation below therefore covers Claude, other hosted vision-language models as a class, local open layout models, and classical methods, and the harness accepts any new candidate as a `Detector` plugin.

| Criterion | Claude (Opus 5.5 default; Sonnet 5.5, Haiku 5.5 selectable) | Other hosted VLMs (OpenAI, Google) | Local layout models (T3) | Structural + CV (T1, T2) |
|---|---|---|---|---|
| Accuracy on semantic calls (headers, marginalia, two-up) | Expected best in class; to be measured in the spike | Comparable class; measured only if the user wants a second vendor | Good where classes exist; no reasoning about intent | None; rule-based |
| Edge precision | Moderate when regressing coordinates; high with propose-and-choose plus snapping | Same caveat | Within a few pixels at 1024 px; snapping fixes the rest | Exact for born-digital; pixel-level for scans |
| Latency per cluster | 2 to 6 s | similar | 0.2 to 6 s (CPU) | < 100 ms |
| Cost per 300-page book (2 to 6 clusters) | roughly 2 to 10 cents at Opus 5.5 prices ($4 per million input tokens, $20 output; an image of about 1.1 megapixels is about 1,600 tokens) | similar order | zero after download | zero |
| Offline | no | no | yes | yes |
| Privacy | page images leave the machine; opt-in only; 30-day retention policies apply per vendor | same | local | local |
| Integration in Go | official SDK, image and PDF blocks, structured outputs (`output_config.format`), prompt caching for the fixed system prompt, Batch API at half price for bulk evaluation | community clients | onnxruntime_go or purego loader plus model download | none |
| Licensing | API terms | API terms | Apache-2.0 weights listed above; avoid AGPL runtimes | n/a |
| Determinism | low; mitigate by making it choose among deterministic candidates | low | high | high |

Recommendation: structural plus CV as the always-on default; local ML as an optional download that most scan users will want; Claude as the opt-in assist and the escalation path for disagreements. Claude is the recommended hosted vendor because the propose-and-choose design plays to VLM strengths, the Go SDK and structured outputs make the integration small, and per-cluster cost is a cent. The harness decides the final ranking with numbers (section 8.5).

### 8.4 The Claude detector in detail
- Model `claude-opus-5-5` by default; the settings UI offers `claude-sonnet-5-5` and `claude-haiku-5-5` for users who prefer lower cost. Adaptive thinking stays on with `effort: low`, which suits a bounded visual choice and keeps latency down.
- Input per cluster: the overlay (gray, long edge 1,568 px at most), two representative pages, and a numbered candidate list drawn on the images. Candidate rectangles come from T0 to T3 plus systematic variants (strip top band, strip bottom band, strip left or right narrow column, split at detected valley).
- Prompt: system text explains the briss task and the rules (never clip body text; headers, footers and page numbers are excluded unless told otherwise; marginal notes count as content unless told otherwise; prefer the tightest rectangle that satisfies the rules). The user turn carries the images and asks for a JSON object `{choice: <candidate id>, adjust: {l,b,r,t} in 0..1000 or null, split: "none"|"columns"|"rows", reasons: string}` enforced with structured outputs. No forced tool choice is used (not supported on this model generation); structured output is the mechanism.
- Post-processing: apply the chosen candidate, apply any adjustment, snap to ink edges, harmonize, clamp. If the response is a refusal or malformed, fall back to the best local candidate and mark the cluster for review.
- Cost control: one request per cluster, at most three images, prompt caching on the system text, a per-document cap (default 20 clusters) and a visible running total in the GUI.
- Privacy: the feature is off until an API key is entered; the settings screen states what is sent; `--detector claude` on the CLI prints the same notice once.
- Evaluation mode: `briss bench --detectors claude` can route through the Batch API to halve the cost of large runs.

### 8.5 Evaluation harness and dataset
Dataset (target 600 to 1,000 labeled cluster images, grown over time):
- **DocLayNet** (CC-BY-4.0, about 80k pages with page-header and page-footer classes): ground-truth crop boxes are derived automatically as the union of all non-header, non-footer boxes, with and without the header band. This gives tens of thousands of labeled born-digital and scanned pages across financial, scientific, legal, manual, and patent documents at no labeling cost.
- **Public-domain scans** (Internet Archive, Project Gutenberg scans, HathiTrust public domain): 150 to 300 clusters hand-labeled in the new GUI, covering two-up scans, skew, black borders, marginalia, and bleed-through.
- **Synthetic generator** (`spike/tools/gen_corpus.py`, Amended): a 290-page book with running headers, page numbers, figures, two-column and margin-note chapters, landscape pages and outlines, plus 118 single-page and 59 two-up scanned-looking pages, all with exact body boxes. It is the first benchmark set and the CI smoke set because its labels are free and exact. It does not replace real scans.
- **Born-digital samples**: arXiv papers, ebook exports, slides, magazines, sheet music, and the three PDFs already in `testdata/`.
- **User contributions**: the opt-in "contribute corrections" export (crop definition plus overlay PNG, no text), curated before inclusion.

Metrics per cluster, reported per detector and per document type:
- mean IoU against ground truth;
- clipped-content rate: any ground-truth content pixel outside the predicted rectangle (hard failure);
- waste: predicted area minus ground-truth area, as a fraction of page area;
- acceptance rate: IoU at or above 0.95 and no clipping (the "accept without editing" proxy);
- split correctness for two-up and column cases;
- latency p50 and p95, and cost for hosted tiers.

Harness: `briss bench` reads the dataset manifest, runs the selected detectors, writes CSV and a Markdown report, and `go test` runs a 50-cluster smoke subset in CI with a regression gate on acceptance rate (no drop above 2 points) and clipped-content rate (no increase).

### 8.6 Decision gates for the AI tier
- Ship T1 and T2 in 1.0 unconditionally.
- Ship T3 if a candidate model reaches at least 85% acceptance on scans with a clipped-content rate below 1%, under 500 ms per page on CPU, and a license in the Apache/MIT family. Measured so far (Amended): the CDLA PicoDet model bundled with the rapid_layout wheel reached 73 to 83% acceptance with ink snapping on the synthetic corpus and fails the gate; the decision waits for the candidates in section 8.7. Ink-edge snapping stays in the post-processing of every tier because it raised scan acceptance from 29 to 33 of 40 at no cost.
- Ship T4 if the Claude spike reaches at least 90% acceptance on the "hard" subset (clusters where T1 to T3 disagree) with zero clipped-content cases, and if cost per cluster stays under 2 cents. Otherwise keep it as an experimental flag.
- Revisit "Jev" or any other named vendor once identified: implement a `Detector`, run the same bench, compare on the same table.

---

### 8.7 Evaluating the candidate layout models (needs model-hub access)
The candidate weights are not on any package index. They live on model hubs: PP-DocLayout_plus-L on Hugging Face under `PaddlePaddle/PP-DocLayout_plus-L` (PaddlePaddle inference format) and, pre-converted to ONNX, in RapidAI's RapidLayout release on ModelScope (`pp_doc_layoutv2`, `pp_doc_layoutv3`); DocLayout-YOLO DocStructBench on Hugging Face (`juliozhao/DocLayout-YOLO-DocStructBench`, PyTorch) and as ONNX on ModelScope (`doclayout_docstructbench`) and in community Hugging Face repositories; docling-layout-heron on Hugging Face (`ds4sd/docling-layout-heron`, safetensors). The cloud session used for the spike reaches PyPI and the Go module proxy but not huggingface.co or modelscope.cn (both are denied by the environment's network policy; the harness-side fetcher cannot resolve them either), so the evaluation has to run on a machine that can: a developer laptop, a CI job with egress to those hosts, or this cloud environment after `huggingface.co` and `www.modelscope.cn` are added under Allowed domains in the environment's network settings (Network access, Limited level, with package managers still allowed). Steps:
1. `pip install rapid-layout onnxruntime pypdfium2 pillow numpy`, then generate the corpus with `python spike/tools/gen_corpus.py spike/testdata`.
2. Run `python spike/tools/onnx_eval.py spike/testdata 40 --model pp_doc_layoutv2` and again with `--model doclayout_docstructbench`. On first use rapid_layout downloads the ONNX file (tens to a few hundred MB) from ModelScope into its cache; `--path file.onnx` points at a file obtained another way. The script excludes the model's header, footer and page-number classes (PP-DocLayout: `header`, `footer`, `number`, `header_image`, `footer_image`; DocLayout-YOLO: `abandon`) and reports latency, IoU, clipping and acceptance with and without ink snapping.
3. For PP-DocLayout_plus-L itself (RT-DETR-L), export the Hugging Face PaddlePaddle checkpoint with `paddle2onnx` and pass the result with `--path`. The heron model needs a small adapter because its RT-DETRv2 output heads differ from the handlers rapid_layout ships.
4. Record the numbers in `spike/RESULTS.md` and apply the gate from section 8.6, with the weight license and checksum recorded next to the result.

## 9. Roadmap

| Phase | Weeks | Deliverables | Exit criteria |
|---|---|---|---|
| 0. Spike | 1 | Done 2026-11-04 (Amended): six spike programs under `spike/`, corpus generator, `spike/RESULTS.md` | Go confirmed; Fyne provisional (section 7.3); layout-model and Claude gates still open (sections 8.6, 8.7) |
| 1. Core library | 2 to 3 | `internal/pdf`, `cluster`, `overlay`, `geom`, `detect/legacy`, `crop`, `cropdef`; Java tree moved to `legacy-java/`; golden tests comparing cluster membership, overlay checksums (tolerance), legacy rectangles (within 0.5%), and output page boxes against the Java build on the same PDFs; fixtures with outlines, named destinations and link annotations for the in-place page-cloning writer (Amended) | `go test ./...` green; golden diffs within tolerance on all `testdata/` PDFs including the encrypted one; outline, named-destination and link targets verified on the cloned pages |
| 2. CLI | 1 to 2 | `briss crop`, `detect`, `preview`, `apply`, legacy flags, JSON crop definitions, goreleaser config, CI matrix (Linux, macOS, Windows; WASM flavour only, cgo optional later, Amended) | Release 0.1 with CLI only; legacy flag behavior byte-for-byte identical on the sample PDFs |
| 3. GUI parity | 3 to 4 | Fyne app with every item in section 7.1; packaging via `fyne package`; screenshots in README | Manual test script covering the Java feature list passes on all three platforms |
| 4. Detection | 3 to 4 (overlaps phase 3 with two engineers) | T1 structural, T2 CV, bench harness, DocLayNet importer, hand-labeled scan set, T3 ONNX detector and `briss model pull`, T4 Claude detector, `auto` policy, GUI integration (Detect buttons, review strip, page strip) | Bench report published; gates in section 8.6 evaluated; `auto` enabled by default |
| 5. Hardening and 1.0 | 1 to 2 | Fuzzing of the PDF and JSON inputs, large-file memory tests, crash reporting (local log only), docs, migration notes, 1.0 release | Performance budgets met in CI; no P1 bugs open |

Later options, not scheduled: `briss serve` local web UI, batch folder processing with a watch mode, deskew via content-stream rewriting, fine-tuned small model trained on the collected dataset, right-to-left and vertical-text policies.

---

## 10. Testing strategy
- **Golden parity tests** (phase 1): the Java build runs in CI (temurin 17) on `testdata/` to produce crop definitions and output PDFs; Go tests compare clusters, rectangles, and output page boxes within tolerance. Retire the Java build only after two releases with no parity regressions.
- **Unit tests** per package, with table tests for rotation, box selection, ratio conversion, header detection, and outline shifting.
- **Property tests**: random rectangles and rotations round-trip through `geom`; random page duplication patterns keep outline targets consistent.
- **Fuzzing**: `go test -fuzz` on the crop-definition parser and on page-box parsing with corrupted PDFs (PDFium runs sandboxed in WASM mode, so crashes are contained).
- **Benchmarks**: section 4.5 budgets as `-bench` tests on fixed inputs with a 20% regression gate.
- **GUI tests**: Fyne's test package for widget logic; a scripted smoke run (open, detect, crop) on each platform in CI using the software renderer.
- **Bench harness** smoke subset in CI as described in section 8.5.

---

## 11. Packaging and distribution
- goreleaser builds: `briss` (WASM PDFium, cgo-free) for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64. A `briss-native` cgo flavour is not part of 1.0 (Amended); the build tag stays so it can be added for single-core machines if users ask. GUI builds need cgo for the window layer on every platform, so the CI matrix uses native runners rather than cross-compilation.
- Local ML support ships as an optional download (`briss model pull`), which fetches the ONNX Runtime shared library for the platform and the chosen model into the user cache directory with checksum verification. The core binary never depends on it.
- macOS: signed and notarized `.app` plus a bare CLI binary. Windows: zip and an MSI via WiX. Linux: tarball, AppImage, and a Flatpak manifest.
- Homebrew tap and Scoop manifest for the CLI.

---

## 12. Licensing
- briss-go stays GPL-3.0-or-later. The clustering, overlay, legacy auto-crop, and split algorithms are ported from GPL-licensed Java and are derivative works. Keep the original copyright notices.
- Dependencies: PDFium (Apache-2.0 and BSD-3-style components), go-pdfium (MIT), wazero (Apache-2.0), pdfcpu (Apache-2.0), Fyne (BSD-3), Cobra (Apache-2.0), ONNX Runtime (MIT), anthropic-sdk-go (MIT). All are GPL-3.0 compatible. iText (AGPL) and PDFBox are no longer used.
- Model weights: ship or download only Apache-2.0 weights (PP-DocLayout_plus-L, DocLayout-YOLO DocStructBench weights, docling-layout-heron). Do not ship AGPL inference code (the `doclayout_yolo` and `ultralytics` runtimes); run exported ONNX graphs through ONNX Runtime instead. Record the license and checksum of every model in `internal/detect/onnx/models.go`.
- Datasets: DocLayNet is CC-BY-4.0 (attribution in the bench report). Public-domain scans only for the hand-labeled set.

---

## 13. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| WASM PDFium too slow or memory-hungry on large books | Misses the budget | Measured inside the budget in the spike (Amended). Remaining risks: the first-run compile (4 to 6 s, mitigated by the compilation cache and background compile) and image-heavy scans with four workers (mitigated by the pool cap in section 4.4); cgo flavour as a last resort |
| Page duplication in pdfcpu breaks outlines, named destinations, or viewers | Wrong bookmarks, unreadable files | In-place cloning validated in the spike for outlines, boxes, metadata and encryption (Amended); named destinations and link annotations still need fixtures; cross-viewer test corpus; keep the Java golden outputs for comparison |
| Fyne editor fidelity (cursors, HiDPI, redraw cost) | Worse UX than the Swing app | Widget and custom diagonal cursors validated headlessly; frame-rate procedure and no-display fallback in section 7.3; Wails fallback with the same core |
| ML runtime distribution on three platforms | Support burden | Optional download, checksummed, never required; WASM-under-wazero escape hatch |
| Model license drift (weights relicensed, AGPL loaders) | Legal exposure | Pin checksums; record licenses; only Apache or MIT weights; no AGPL runtimes |
| VLM gives imprecise coordinates or clips text | Users lose trust in Detect | Propose-and-choose plus snapping; clipped-content rate is a hard gate; review strip always shows the result before cropping |
| Privacy concerns about sending pages to a cloud API | Adoption | Opt-in, clear notice, local tiers always available, no telemetry by default |
| Scope creep in the GUI | Schedule | Parity list in section 7.1 is the definition of done; additions in 7.2 are time-boxed |
| Rotated pages and unusual box combinations | Wrong crops | Port `RectangleHandler` logic exactly; property tests over rotation and box choice |
| PDFium crash on a malformed file | App crash | WASM sandbox contains it; cgo flavor uses the multi-threaded subprocess mode for the GUI |

---

## 14. Open questions for the project owner
1. **"Jev"**: the brief lists "Jev, Claude, or other" as candidate tooling. No tool, model, or vendor by that name could be identified. If it is a specific product (or a typo for one), name it and it gets a `Detector` adapter and a row in the bench report.
2. **Second hosted vendor**: should the harness also measure an OpenAI or Google vision model for comparison, at the cost of two more API integrations, or is Claude the only hosted candidate?
3. **cgo release flavor**: answered by the spike (Amended): the WASM build meets the budget and four workers match native cgo on wall-clock, so the cgo flavour is optional and not shipped in 1.0.
4. **Per-page cropping default**: should scanned documents default to per-page rectangles (better fit, varying page sizes in the output) or keep the cluster-wide rectangle (stable geometry, as Briss has always done)?
5. **Dataset contributions**: is an opt-in export of anonymized crop definitions and overlay images acceptable for building the public benchmark?
6. **Java tree**: keep it in `legacy-java/` in this repository until parity, or move it to a `java` branch and reference it from CI by commit?
7. **Model-hub access** (Amended): who runs the layout-model evaluation in section 8.7, and on which machine with access to huggingface.co or modelscope.cn?
8. **Display for the frame-rate check** (Amended): does anyone on the team have a desktop, at any resolution, to run the section 7.3 procedure during phase 3? If not, the no-display path in 7.3 applies.
