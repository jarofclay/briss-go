# Week-1 spike results

Date: 2026-11-04. Machine: 4 vCPU cloud VM, 15 GB RAM, Go 1.26 (auto-selected by
go-pdfium v1.21.4), no GPU, no display. Corpus: `spike/tools/gen_corpus.py` (seed 42):
`book300.pdf` (290 pages, born-digital, running headers, page numbers, figures, a
two-column chapter, margin notes, 4 landscape pages, 26 outline entries),
`scan_single.pdf` (118 scanned-looking pages with skew, offset, noise, gutter shadow),
`scan_twoup.pdf` (59 two-up sheets). Every page carries an exact ground-truth body box.
Public PDFs could not be downloaded in this environment (GitHub releases, Hugging Face,
ModelScope and arXiv are blocked), so the corpus is synthetic; the numbers below are
indicative and must be re-run on real books before 1.0.

## Verdict against the exit criteria (PLAN.md section 3.4)

| # | Spike | Criterion | Result | Verdict |
|---|---|---|---|---|
| 1 | go-pdfium WebAssembly | p50 < 120 ms per page per core; pool startup < 1.5 s; peak RSS < 400 MB | born-digital p50 10 to 15 ms, scans p50 53 to 75 ms, two-up p50 67 to 124 ms (4 workers, 900 px tall, gray); startup 92 to 130 ms with wazero's compilation cache (4.8 s cold on first ever run); peak RSS 126 MB (book), 241 MB (scans), 281 MB (two-up) with 4 workers, 200 to 212 MB with 2 | **Pass** (first-run compile must be handled, see notes) |
| 2 | go-pdfium cgo | record speedup | per page 2.3 to 2.7x faster than WASM (book p50 4 ms, scans 23 to 28 ms), startup 0 s, RSS 71 to 105 MB; but the single-threaded backend serialises calls, so wall-clock for 30 scan pages was 764 to 828 ms versus 644 to 996 ms for 4 WASM workers | **Measured**; cgo flavour is optional, not required for the budget |
| 3 | pdfcpu page cloning | opens in viewers; bookmarks on the right pages; size within 5% of Java | 290 -> 314 pages (24 column clones); 0 text/box mismatches; 26/26 outline targets correct without any renumbering; metadata kept; encrypted source stays encrypted with the same password; output is 89% of the source size and 11 to 21% smaller than the Java output; read+apply+write 33 to 45 ms | **Pass** (named destinations not exercised: the generator produced none; add to phase-1 tests) |
| 4 | Fyne crop editor | draggable, resizable rectangles with custom cursors at 60 fps on 4K | widget implemented (draw, move, eight resize handles with custom diagonal cursors, select, multi-rect, delete, keyboard nudge, ghost rectangle); headless test passes on the software driver; frame cost on the software renderer 75 ms (full-canvas software capture dominates; not representative of the GPU driver); GPU frame rate needs a desktop | **Pass headlessly; the 60 fps / 4K check is a desktop task** (`cd spike/gui && go run . ../testdata/overlays/book300_c00_odd_612x792_p1.png`) |
| 5 | ONNX layout model on CPU | p50 < 400 ms per 1024 px page | CDLA PicoDet (the only layout model obtainable here): raw session.run p50 53 to 64 ms at 800x608, end-to-end p50 93 to 105 ms including resize and NMS | **Pass on latency**; accuracy below (the Apache-2.0 candidates from the plan still need to be measured) |
| 6 | Claude propose-and-choose | mean IoU > 0.93, zero clipped, p50 < 6 s on 30 labelled clusters | harness complete (candidate generation, numbered overlays, structured JSON output, scoring); no API key in this environment, so only dry and mock modes ran; local candidate sets contain a rectangle with IoU 0.95 to 0.99 against ground truth for all 8 clusters, which is the ceiling the model has to pick from | **Blocked on credentials**; run `cd spike/claude && ANTHROPIC_API_KEY=... ./claude-spike -mode live` |

Decision: **Go confirmed.** Nothing in the rendering, writing, or GUI spikes points at Rust.
Fyne confirmed for the editor subject to the desktop fps check. The Claude gate and the
real-document accuracy numbers remain open.

## Spike 1 and 2: rendering

Setup: cluster pages like Briss (parity, size within 2 pt), sample at most 15 pages per
cluster, render each sampled page directly to gray at 900 px tall, accumulate a
streaming mean/variance overlay (same formula as `ClusterImageData`), write PNGs.
Rendering at the target size instead of 300 DPI RGB is a 13x reduction in pixels
versus the Java pipeline.

WebAssembly backend, compilation cache warm, 4 workers:

| File | Clusters / rendered pages | Render p50 / p95 | Wall for all renders | Peak RSS |
|---|---|---|---|---|
| book300 (born-digital) | 4 / 34 | 10 to 15 ms / 72 to 93 ms | 215 to 271 ms | 126 MB |
| scan_single (JPEG pages) | 2 / 30 | 75 ms / 124 ms | 644 ms | 241 MB |
| scan_twoup (large JPEG sheets) | 2 / 30 | 124 ms / 215 ms | 996 ms | 281 MB |
| CREATIVE_COMMONS (repo sample, slides) | 2 / 28 | 26 ms / 86 ms | 312 ms | 200 MB |

With 2 workers the scan p50 drops to 53 ms (two-up 67 ms) because the four WASM
instances contend for the four vCPUs; wall-clock is slightly longer. Startup is the
dominant fixed cost: 3.6 to 5.8 s to compile the embedded PDFium module without a
cache, 92 to 130 ms with `wazero.NewCompilationCacheWithDir` (21 MB on disk). The cache
must be created with the same core features go-pdfium enables
(`api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling`), otherwise the
module fails to compile.

cgo backend (PDFium 8076 from the pypdfium2 wheel, headers from the chromium/pdfium
mirror), single-threaded:

| File | Render p50 / p95 | Wall | Peak RSS |
|---|---|---|---|
| book300 | 4 ms / 15 ms | 192 ms | 102 MB |
| scan_single | 23 ms / 33 ms | 764 ms | 71 MB |
| scan_twoup | 28 ms / 32 ms | 828 ms | 105 MB |

Observations:
- Per page, native is 2.3 to 2.7x faster; across four cores the WASM pool matches or
  beats it on wall-clock. A cgo flavour is worth having only for single-core machines or
  very large scans, and it needs matching PDFium headers (`pdfium_experimental` failed to
  build against the mirror's `main` headers because newer attachment and catalog
  functions were missing).
- Encrypted PDFs open with a password in both backends.
- End-to-end for book300 (open, cluster, render 34 pages, overlays, write cropped PDF)
  is about 0.35 s in Go versus 6.1 to 9.2 s for the Java CLI on the same machine.

Open item: ship or pre-warm the wazero cache on first start (background compile while
the start screen shows, or compile at install time), and verify behaviour on arm64 and
on machines where the cache directory is not writable.

## Spike 3: pdfcpu writer

Approach: read and validate with pdfcpu, resolve every page dictionary first, then for
each page with k rectangles keep the dictionary for rectangle 1 and insert k-1 shallow
clones after it in the parent's `/Kids` (sharing `/Contents`, `/Resources`, `/Annots` by
reference), bump `/Count` up the tree, set `/MediaBox` and `/CropBox` with the Java
`RectangleHandler` maths (smallest of media and crop box, ratios rotated by `/Rotate`),
then write. Because page objects keep their identity, every outline item, named
destination and link that pointed at a page still points at its first copy. This
replaces the Java two-pass copy plus bookmark renumbering.

| Check | Result |
|---|---|
| Page count | 290 -> 314 with 24 clones on the two-column chapter |
| Text of each output page equals its source page; clones have identical text and non-overlapping column boxes | 0 mismatches |
| Outline targets | 26 of 26 land on the expected output page |
| Metadata | Title, Author, Subject, Keywords kept; Producer becomes pdfcpu |
| Encrypted source (`example-protected.pdf`, password `secret`) | output still encrypted, opens only with the password, boxes applied |
| Size | 576 KB from a 645 KB source (89%); Java iText output was 651 KB (one rectangle) and 726 KB (two rectangles per page) |
| Time | read+validate 18 to 22 ms, apply 5 to 7 ms, write 8 to 12 ms |

Open items: named destinations were not exercised (reportlab emitted none; create a
fixture with pdfcpu's bookmark import and with a `/Dests` tree), pdfcpu's validation
strictness on malformed real-world PDFs (use relaxed mode and keep a corpus of odd
files), and link annotations shared between clones (acceptable, but a link rectangle
outside a clone's crop box is simply invisible).

## Spike 4: Fyne editor

`spike/gui/editor.go` is a custom widget on `widget.BaseWidget` implementing
`Draggable`, `Tappable`, `SecondaryTappable`, `desktop.Hoverable`,
`desktop.Cursorable` and `Focusable`. Fyne has no diagonal resize cursors, so the
corner handles use custom 24x24 cursor bitmaps (`desktop.Cursor` is an interface, so
this works without patching Fyne). The renderer rebuilds canvas rectangles, handle
squares and a size label per crop rectangle on every change; the image is a
`canvas.Image` with `ImageScaleFastest`.

The headless test drives draw, move, corner resize, tap-select and keyboard nudges
through the software driver and writes `spike/testdata/gui/editor_headless.png`.
Frame cost (refresh plus software capture of a 695x900 editor with 2 rectangles):
75 ms per cycle, of which the software raster of the whole 695x900 canvas is the bulk (the widget rebuild itself is sub-millisecond: 23 canvas objects). The software renderer is not what users see; the GPU driver
is expected to be much faster, but the 60 fps on 4K criterion has to be checked on a
real desktop. Building Fyne on Linux needs `libgl1-mesa-dev`, `xorg-dev`,
`libxkbcommon-dev`, `libwayland-dev` and `wayland-protocols`; the resulting binary is
32 MB unstripped.

Risks seen: one drag semantic difference between the real driver (first `Dragged`
carries the pointer position after the first motion plus the delta) and `test.Drag`
(position is the press point); the widget follows the real driver and the test
simulates it. Keep this in mind for GUI tests. Wails remains the fallback if the desktop
fps check fails.

## Spike 5: local layout model

Only one layout model was obtainable offline: the CDLA PicoDet model bundled in the
`rapid_layout` wheel (10 classes including `header` and `footer`, input 800x608). The
Apache-2.0 candidates from the plan (PP-DocLayout_plus-L, DocLayout-YOLO DocStructBench,
docling heron) are hosted on Hugging Face and ModelScope, both blocked here.

| Corpus (40 pages each) | Latency p50 / p95 | Mean IoU | Clipped (>1% of page) | Accept (IoU >= 0.95, no clip) |
|---|---|---|---|---|
| book300, raw boxes | 105 ms / 130 ms | 0.968 | 5 / 40 | 31 / 40 |
| scan_single, raw boxes | 93 ms / 111 ms | 0.966 | 11 / 40 | 29 / 40 |
| book300, boxes snapped to ink edges (24 pt, blocked by detected header/footer) | 98 ms / 136 ms | 0.968 | 5 / 40 | 31 / 40 |
| scan_single, snapped | 106 ms / 136 ms | 0.978 | 7 / 40 | 33 / 40 |

The raw ONNX Runtime call is 53 to 64 ms on 4 threads, so a sampled cluster of 15 pages
costs about 1.5 s on CPU. Accuracy of this small model alone (73 to 83% acceptance with
snapping) is below the 85% gate in the plan; the remaining clips are last body lines
the model leaves out of its text blocks and column-split pages. Snapping to ink edges
helps scans and is cheap, which supports the hybrid design. The gate decision needs the
larger Apache-2.0 models on a machine with model-hub access.

Note: an early run showed 40/40 clipped; that was a corpus label bug (trailing blank
spacer lines extended the ground-truth body box below the last inked line) and is fixed
in the generator.

## Spike 6: Claude harness

`spike/claude` builds candidates from the overlay: the Java legacy auto-crop port, the
ink bounding box, the box without the top band, without the bottom band, without both,
and without narrow side columns (only distinct variants are kept, 3 to 5 per cluster).
It draws them numbered on the overlay and two sample pages, sends the images with a
cached system prompt and a strict JSON schema (`output_config.format`, effort `low`,
model `claude-opus-5-5`), parses `{choice, adjust, split, reasons}`, snaps, and scores
IoU and clipping against ground truth; it also reports latency, token counts and cost.

Local ceiling (best candidate per cluster versus ground truth): book 0.991 (portrait)
and 0.946 (landscape figure pages), scans 0.952 to 0.960, two-up 0.973. The Java legacy
rectangle alone scores 0.963 on born-digital pages but 0.85 to 0.86 on scans because it
keeps the header band. So the choice the model has to make is real, and a correct pick
clears the 0.93 gate on every cluster. The live run and the 30-cluster labelled set are
outstanding; the two-up overlay and the margin-note cluster are the interesting cases
to watch (the annotated images are in `spike/testdata/claude-run/` after a dry run).

## What changes in PLAN.md

1. Startup budget: add "first-run WASM compile 4 to 6 s" as a known cost; pre-warm or
   precompile.
2. Memory budget: 4 workers on large scanned sheets reach 281 MB on this corpus; cap the
   pool by memory and prefer 2 to 3 workers on 8 GB machines.
3. The cgo flavour is optional for 1.0.
4. Bookmark handling in the writer is simpler than planned: no renumbering is needed with
   in-place cloning.
5. Add named-destination and link-annotation fixtures to the phase-1 golden tests.
6. ML tier: the CDLA PicoDet model does not pass the accuracy gate; evaluate
   PP-DocLayout_plus-L and DocLayout-YOLO before committing to a model, and keep ink
   snapping in the post-processing for every tier.
7. Corpus: the synthetic generator with exact labels is worth keeping as the first
   benchmark set; real scans still need to be added.
