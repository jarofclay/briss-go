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
| 6 | Claude propose-and-choose | mean IoU > 0.93, zero clipped, p50 < 6 s on 30 labelled clusters | Live on 8 clusters (2026-11-05, test key, see the Spike 6 section): choice-only with one panel per candidate gives Opus 5.5 mean IoU 0.967, 0/8 clipped, p50 5.9 s; Sonnet 5.5 0.961, 1/8 flagged at the 1% tolerance, p50 2.9 s; Haiku 5.5 0.961, 1/8, p50 3.3 s. Letting the model adjust coordinates was harmful for every model (Opus 0.851 with 2 clips, Haiku 0.56 to 0.69 on several clusters) | **Pass for choice-only** on this corpus; the 30-cluster real-document set is still to be built |

Decision: **Go confirmed.** Nothing in the rendering, writing, or GUI spikes points at Rust.
Fyne confirmed for the editor subject to the desktop fps check. The Claude tier passes in
choice-only form on the synthetic corpus; the real-document accuracy numbers remain open.

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

Addendum 2026-11-05: after `huggingface.co` and `www.modelscope.cn` were allowed in the
environment, the model pages became reachable but the downloads still fail one hop later:
Hugging Face redirects file downloads to `us.aws.cdn.hf.co` and ModelScope to
`cdn-lfs-cn-1.modelscope.cn`, both denied by the proxy. Those CDN hosts need allowing
too (`cdn-lfs.hf.co` and `cas-bridge.xethub.hf.co` are other hosts Hugging Face uses).
The `PaddlePaddle/PP-DocLayout_plus-L` repository (Apache-2.0) holds Paddle inference
files (`inference.json`, `inference.pdiparams`, `inference.yml`), so it needs a
`paddle2onnx` export before `onnx_eval.py --path` can use it; the community
`podongchip/DocLayout-YOLO-DocStructBench-ONNX` repository (Apache-2.0) has a ready
`doclayout_yolo_docstructbench_imgsz1024.onnx`.

## Spike 6: Claude harness

`spike/claude` builds candidates from the overlay: the Java legacy auto-crop port, the
ink bounding box, the box without the top band, without the bottom band, without both,
and without narrow side columns (only distinct variants are kept, 3 to 5 per cluster).
It draws them numbered on the overlay and two sample pages (or, with `-panels`, one
clean overlay per candidate plus clean sample pages), sends the images with a cached
system prompt and a strict JSON schema (`output_config.format`, effort `low`,
`max_tokens` 8192), parses `{choice, adjust?, split, reasons}`, and scores IoU and
clipping against ground truth with latency, tokens and cost.

Local ceiling (best candidate per cluster versus ground truth): book 0.991 (portrait)
and 0.946 (landscape figure pages), scans 0.952 to 0.960, two-up 0.973. The Java legacy
rectangle alone scores 0.963 on born-digital pages but 0.85 to 0.86 on scans because it
keeps the header band.

Live runs on 2026-11-05 with a test key passed through the environment (8 clusters per
run, prices at each model's own list rate, cache reads not counted):

| Variant | Model | Mean IoU | Clipped (>1%) | Latency p50 | Cost per cluster |
|---|---|---|---|---|---|
| choose + adjust, one annotated overlay | Opus 5.5 | 0.851 | 2 / 8 | 4.1 s | 1.6 c |
| choose + adjust, one annotated overlay | Sonnet 5.5 | 0.944 | 4 / 8 | 2.5 s | 0.8 c |
| choose + adjust, one annotated overlay | Haiku 5.5 | 0.80 (3 clusters with IoU 0.56 to 0.69) | 0 / 8 | 8.5 s | 0.1 c |
| choice only, one annotated overlay (nested outlines inset 3 px per id) | Opus 5.5 | 0.903 | 0 / 8 | 3.3 s | 1.6 c |
| choice only, one annotated overlay | Sonnet 5.5 | 0.933 | 0 / 8 | 2.6 s | 0.8 c |
| choice only, one annotated overlay | Haiku 5.5 | 0.961 | 1 / 8 | 1.7 s | 0.04 c |
| **choice only, one panel per candidate** | **Opus 5.5** | **0.967** | **0 / 8** | 5.9 s (one 57 s outlier that hit the token cap) | 5.1 c (3.0 c without the outlier) |
| choice only, one panel per candidate | Sonnet 5.5 | 0.961 | 1 / 8 | 2.9 s | 1.5 c |
| choice only, one panel per candidate | Haiku 5.5 | 0.961 | 1 / 8 | 3.3 s | 0.08 c |

What the runs showed:
- Free-form coordinate adjustment is the weak link. Opus wrote `t: 900` for a box whose
  top sits 90 px from the top of a 900 px image, that is a pixel coordinate where
  thousandths-from-the-edge were asked for, and Haiku's adjustments wandered by 10 to 30%
  of the page. Every model picked the right candidate far more reliably than it could
  move an edge. Choice-only is the design; precision comes from the local candidates and
  ink snapping, which is what the plan intended.
- The first drawing style penalised tight candidates: nested outlines were inset 3 px per
  id so they stayed visible, which made candidate 4 look as if it cut into text. Opus,
  which takes rule 1 (never clip) most seriously, then chose looser boxes. One clean
  panel per candidate fixed that and all three models converged on the same choices.
- Thinking tokens count against `max_tokens` on the 5.5 generation. With 1,024 tokens
  Haiku truncated 5 of 8 answers; 8,192 fixed it. Opus once spent the whole 8,192 budget
  on a two-up cluster (57 s); the answer still parsed, but a per-request timeout and a
  retry at a lower cap belong in the product.
- The one "clip" flagged for Sonnet and Haiku is the even-page book cluster where the
  chosen candidate strips the bottom band and the cluster median includes figure
  captions on some pages; it is at the 1% tolerance boundary (IoU 0.946), the same box
  Opus chose on the odd cluster without a flag.
- Panel mode sends 5 to 7 images (4,300 to 9,900 input tokens per cluster), so cost is
  three times the single-image estimate in the plan: about 3 c per cluster on Opus 5.5
  (5 c with the outlier), 1.5 c on Sonnet 5.5, under 0.1 c on Haiku 5.5. A 300-page book
  with 2 to 6 clusters costs 6 to 30 c on Opus and 3 to 9 c on Sonnet.

Gate from the plan (mean IoU above 0.93, zero clipped, p50 under 6 s): Opus 5.5 passes
outright; Sonnet 5.5 and Haiku 5.5 pass on IoU and latency with one borderline flag each.
Recommendation: keep `claude-opus-5-5` as the default for the assist tier because
"never clip" is the hard rule, offer Sonnet 5.5 as the budget option in settings (same
choices on this corpus at a third of the cost), and revisit on the real-document set.
The 30-cluster real-document set from the plan is still to be built.

The test key was supplied for this session only, was never written into the repository
or any tracked file, and should be revoked now that the runs are done.

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
8. Claude tier (2026-11-05): choice-only with one panel per candidate, no free-form
   coordinate adjustment, `max_tokens` 8192 with a timeout and lower-cap retry, cost
   budget 3 to 5 c per cluster on Opus 5.5 or 1.5 c on Sonnet 5.5.
9. Secrets: API keys only through the environment or the OS keychain, never in the
   repository (public fork); `.gitignore` covers `.env`, `*.key`, `secrets/`.

## Addendum 2026-11-05: frame rate, DocLayout-YOLO, real PDFs

**Frame rate.** The spike editor's `BRISS_FRAME_STATS=1` hook now also records the
painted-frame interval (a 1x1 `canvas.Raster` whose generator runs at paint time). Fyne
paints on a 60 Hz ticker, so the original "p95 under 16 ms" criterion was impossible;
the corrected criterion is paint-interval p95 at or under 20 ms. Without a GPU, on Xvfb
with Mesa llvmpipe and xdotool driving drags:

| Display | Window | Paint p50 | Paint p95 | Windows passing |
|---|---|---|---|---|
| 1920x1080, 100% | 900x1000 | 16.4 to 16.9 ms | 17.6 to 19.2 ms (32 to 34 ms in 2 windows) | 9 of 11 |
| 3840x2160, 200% (`FYNE_SCALE=2`) | 1800x2000 | 20 to 27 ms | 31 to 41 ms | 0 of 16 |

The 100% result is strong evidence for real GPUs; the 200% result is CPU rasterisation of
3.6 megapixels per frame and inconclusive. A GPU desktop run remains the decision point
(`docs/decisions/0004-fyne-gui.md`).

**DocLayout-YOLO DocStructBench.** Downloaded from the community repository
`podongchip/DocLayout-YOLO-DocStructBench-ONNX` (Apache-2.0, 76,980,883 bytes, sha256
`dcb395095ba65156eee4e928cefea726454571d487421e4a4ba1a4bd112e498e`; a different export
from the one in the rapid_layout manifest). It uses Ultralytics metadata and an
end-to-end head (`[1, 300, 6]` = box, score, class), so `onnx_eval.py --format yolo-e2e`
has its own letterbox adapter. Crop = union of all boxes except class `abandon`.

| Setting | Corpus (40 pages) | Latency p50 / p95 | Mean IoU | Clipped | Accept |
|---|---|---|---|---|---|
| fp32, conf 0.25 | scan_single | 992 ms / 1147 ms | 0.988 | 0 | 40 / 40 |
| fp32, conf 0.25 | book300 | 928 ms / 1154 ms | 0.978 | 1 | 35 / 40 |
| fp32, conf 0.4 | scan_single | 930 ms / 1061 ms | 0.987 | 1 | 39 / 40 |
| fp32, conf 0.4 | book300 | 1035 ms / 4254 ms | 0.978 | 1 | 35 / 40 |

Accuracy clears the 85% / under-1%-clipping gate on the synthetic scans; latency (raw
session 774 to 958 ms at 1024x1024 on 4 threads) misses the 500 ms gate by about 2x.
Ink snapping did not change acceptance for this model. Dynamic int8 quantization
(ONNX Runtime `quantize_dynamic`, Conv and MatMul, 24 MB) is slower, not faster, on this
CPU: 1.47 s against 0.98 s per inference on identical input, with the same accuracy on a
4-page check (IoU 0.990). Static QDQ quantization with calibration data, a 640 px export
or fewer sampled pages per cluster are the remaining speed options.

**Real PDFs.** `raw.githubusercontent.com` is reachable, so the 38 CC-BY-SA files of
`py-pdf/sample-files` (LaTeX book of 117 pages, multicolumn, outlines, links, forms,
attachments, rotated and scaled pages, an encrypted file, unusual image filters) went
through the whole Go pipeline with the new opt-in `TestRealPDFs`: 37 pass with page
counts, outlines, named destinations and links intact, 1 is skipped because the file has
no `/Root` and PDFium cannot open it (pypdf rejects it too). The 117-page book is
analysed in 0.44 s and cropped end to end in 0.9 s by the CLI; the legacy crop removes its
running header and page number and keeps every body line.
