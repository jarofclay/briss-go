# 0004 Build the GUI with Fyne

Status: provisional 2026-11-05 (PLAN.md section 7.3).

**Decision.** Fyne with a custom crop-editor widget; Wails is the fallback.

**Evidence so far.**
- The spike widget (draw, move, eight resize handles, custom diagonal cursors, selection,
  keyboard) passes a headless test.
- Fyne paints on a 60 Hz ticker, so the useful measure is the painted-frame interval: p95 at
  or under 20 ms means the editor keeps up. (The first version of the criterion, p95 under
  16 ms, cannot be met by any Fyne app and was corrected.)
- Xvfb with Mesa llvmpipe (software rendering on 4 vCPUs), 900x1000 window at 100%: p50
  16.4 to 16.9 ms, p95 17.6 to 19.2 ms in 9 of 11 windows of 60 frames, 32 to 34 ms in 2.
  The editor keeps 60 fps on a CPU renderer with occasional single dropped frames.
- Same on a virtual 3840x2160 display at 200% scaling (1800x2000 window): p50 20 to 27 ms,
  p95 31 to 41 ms, about 40 fps. Software rasterisation of 3.6 megapixels per frame says
  little about a GPU, so this is inconclusive rather than a failure.

**Reopen if** the procedure in PLAN.md section 7.3, run on a real desktop with a GPU,
shows p95 above 20 ms after the two cheap fixes listed there.
