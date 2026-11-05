# 0003 Write with pdfcpu by cloning pages in place

Status: accepted 2026-11-04; implemented in `internal/crop`.

**Decision.** For a page with k crop rectangles keep its dictionary for the first and
insert k-1 shallow clones after it (sharing contents, resources and annotations), then set
`/MediaBox` and `/CropBox` with the Java `RectangleHandler` maths.

**Why.** Original page objects keep their identity, so outlines, named destinations (both
the `/Names` tree and the old `/Dests` dictionary) and links keep pointing at the first
copy of their page without any renumbering. Verified on a fixture with all of these and a
rotated page (`internal/crop`), on the Java golden outputs, and on 37 real PDFs
(`TestRealPDFs`). Output is smaller than the Java output (89% of the source on the book).

**Costs.** pdfcpu validates on read; files that pdfcpu rejects but PDFium renders would
need relaxed handling. None of the 37 real sample files hit this.
