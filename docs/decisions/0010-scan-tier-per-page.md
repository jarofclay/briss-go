# 0010 The scan tier works page by page, from text lines

Status: accepted 2026-11-05; implemented in `internal/detect/cv`, measured in
`spike/RESULTS.md` (addendum "the scan tier on real scans").

**Decision.** Tier T2 (`cv`) detects every page on its own image instead of the
cluster's overlay, and builds each page's block from text lines and figures rather
than from projection profiles:

- ink mask: Otsu threshold after flattening wide, smooth binding shadows column by
  column; scanner borders trimmed; specks, thin lines, border junk and book-edge
  stripes removed; glyph-sized marks cut by the border kept;
- marks side by side on one baseline form text lines; groups with a large solid mark
  are figures; short lines that stand beside the text, off its baselines, are dropped;
- the block is the extent of those lines and figures, minus small clusters at the
  very top or bottom of the sheet, plus glyph-sized marks on clean paper just above or
  below it and baseline-aligned marks beside it;
- landscape sheets split at the empty column run in the middle 30% of the width,
  preferring the binding shadow when several runs qualify; a side with no text line
  is blank and gets no rectangle;
- `auto` picks `cv` when most sampled pages of a cluster are scans (one image covering
  at least 60% of the page), otherwise `legacy` until T1 exists.

**Why.** On real photocopied books each sheet sits differently on the glass, so a
cluster overlay is a blur and the legacy tier accepted 0 of 171 sheets. Profile bands
were stretched by stray marks, stacked page edges and shadow speckle; line grouping
ignores them because they never line up into text. Measured against draft labels: 78.4%
of 171 real sheets accepted, mean IoU 0.973, against 76.0% for PP-DocLayoutV3 and 0%
for legacy, in 10 s for all sheets on 4 threads.

**Cost.** Many parameters, set on these ten books and the synthetic corpus; 12.3% of
real sheets still clip by more than 1%, so the GUI has to present the result for review.
Rendering and detecting every page at 1400 px costs about 60 ms per sheet on 4 threads.

**Reopen if** owner-reviewed labels or a larger scan set show the parameters overfit
(acceptance falling well below the layout models), or T3 becomes fast enough to run on
every page, in which case `auto` should combine the two.
