# 0009 Keep the legacy tier bit-compatible, fix quirks elsewhere

Status: accepted 2026-11-05; tested in `internal/pipeline/golden_test.go`.

**Decision.** Tier T0 reproduces the Java `CropFinder` and `SplitFinder` exactly,
including float32 arithmetic and the row-split quirk (Java searches rows with the overlay
width). The surrounding pipeline fixes Java quirks that are not part of the algorithm:

| Java behaviour | Go behaviour |
|---|---|
| Pages bucketed into 20 pt size bins (near-identical sizes on either side of a bin edge split) | Size tolerance window of +-10 pt |
| Clusters by unrotated page size while previews are rendered rotated | Clusters by displayed size |
| A degenerate auto-crop (zero height) is written into the PDF | The page is kept whole, with a note |
| `-d` pointing at a directory fails | Writes `<name>_cropped.pdf` inside the directory |
| Very wide pages can throw in the row split | Search range clipped to the profile |

**Evidence.** On the Java golden data the port reproduces every auto-crop and split ratio
bit for bit when fed Java's own overlay; cluster membership, sampling and overlay sizes
match exactly; the box maths matches `RectangleHandler` on 64 cases and on every Java
output page. With PDFium rendering instead of PDFBox, legacy ratios stay within 0.33% on 6
clusters; on one slide cluster the "26 of 30" rule flips on a sub-gray-level difference
and the right edge moves by 2.6%. The improved split (inside the rectangle, correct axis)
belongs to tier T2.
