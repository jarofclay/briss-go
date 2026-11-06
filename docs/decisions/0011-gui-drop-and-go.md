# 0011 The app is drop-and-go, dark and minimal, with review as an option

Status: accepted 2026-11-06 at the owner's request; implemented in `gui/` on branch
`claude/gui`.

**Decision.** The desktop app departs from the Java Briss screens (cluster grid first,
crop button last). It opens on a single drop zone; a dropped PDF is cropped and saved
next to the original with no further questions. Everything else sits behind one options
button and is remembered between runs:

- mode: Auto (default), Preview (a preview window, saved on Save), Preview only;
- margin in points;
- same page size (on), selectable text by OCR (on), straighten (on), keep headers (on).

The look is a fixed dark theme (near-black surfaces, one blue accent, line icons, few
words). The preview shows one sheet at a time with editable crop boxes (drag edges,
corners or the inside, draw new, right-click to remove, reset) and the resulting pages,
straightened and cut, beside it. The three new steps live in the shared core
(`internal/app`, `internal/crop`, `internal/ocr`) so the command line offers the same
results (`--straighten --normalize --ocr`).

The app is its own Go module (`gui/`), because Fyne needs cgo and OpenGL while the
command line stays a pure-Go cross-compiled binary. CI (`.github/workflows/gui.yml`)
builds the app on each system so users do not need a C compiler.

**Why.** The owner wants something that works without reading a manual: most PDFs need
no adjustment, and the scan detector, the straightening and the OCR run without
choices. Reviewing stays one switch away for the files that need it.

**Cost.** The Java app's cluster grid, per-cluster editing across many pages at once,
and the copy, align and equalise tools of PLAN.md section 7.1 are not in this design;
the preview edits page by page. OCR depends on a separately installed Tesseract, and
its quality follows the scan (curled pages near the spine read poorly).

**Reopen if** users need to fix many pages of one book at once (bring back a per-cluster
view in the preview), or the unsigned downloads prove too awkward (sign and notarise the
macOS app, sign the Windows executable).
