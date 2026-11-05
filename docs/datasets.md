# Labelled datasets for `briss bench`

`briss bench` scores detectors against PDFs whose correct crop is known. This page
describes the label format and where labelled files should live.

## Keep datasets out of this repository

This repository is a public fork. Only commit PDFs you are allowed to redistribute
(the files in `testdata/pdfs` are; the synthetic corpus is generated). Keep other
datasets in a private place and point `briss bench --dataset` at a local copy:

- a private GitHub repository (Git LFS for files over 100 MB), attached to a Claude
  Code session next to this one;
- a folder on your own computer, which a cloud session can use through Remote
  Control where that is available;
- any local directory when running briss yourself.

## Layout

A dataset is a directory tree. A PDF is labelled when a crop definition sits next to
it with the same name and the suffix `.crops.json`:

```
labels/
  novel.pdf
  novel.crops.json
  scans/journal-1923.pdf
  scans/journal-1923.crops.json
  unlabelled.pdf          <- reported as skipped
```

Encrypted PDFs are fine: pass the passwords to try with `--password` (repeatable).

## Label format

A label is a crop definition, version 1 (`internal/cropdef`, PLAN.md section 6), the
same file `briss detect --save-crops` and `briss crop --save-crops` write. Rectangles
are fractions of the displayed page cut away from the left, bottom, right and top
edges (`l`, `b`, `r`, `t`); `{0,0,0,0}` keeps the whole page.

```json
{
  "version": 1,
  "source": {"path": "novel.pdf", "sha256": "…", "pages": 312},
  "options": {},
  "clusters": [
    {"id": 0, "pages": [1, 3, 5], "even": false, "box_pt": [612, 792],
     "rects": [{"l": 0.12, "b": 0.09, "r": 0.11, "t": 0.08}]}
  ],
  "page_overrides": {
    "17": [{"l": 0.10, "b": 0.09, "r": 0.11, "t": 0.08}],
    "18": [{"l": 0.08, "b": 0.10, "r": 0.52, "t": 0.08}, {"l": 0.52, "b": 0.10, "r": 0.08, "t": 0.08}],
    "1": []
  }
}
```

- Cluster rectangles apply to every page of the cluster; `page_overrides` replace them
  for single pages. Labels can use only `page_overrides` (one entry per page), which is
  easiest when converting from another tool.
- Several rectangles on a page mean the page should be split (columns, two-up scans),
  in output order.
- An empty override (`"1": []`) means the page must not be cropped.
- Pages with no rectangle anywhere in the label are not scored.
- `options.keep_headers: true` says the rectangles include running headers, footers
  and page numbers; `briss bench` then runs every detector with headers kept.
- `source.pages` must equal the PDF's page count; `source.sha256` is optional and only
  produces a warning when it differs.

The quickest way to make a label is to let briss guess and fix the result:

```bash
briss detect novel.pdf --save-crops novel.crops.json
```

then edit the numbers (or, from phase 3, adjust the rectangles in the editor and save).

## Scores

For each labelled page the predicted rectangles are compared with the label:

| Measure | Meaning |
|---|---|
| IoU | Intersection over union; with several rectangles on both sides each pair is compared in order and the worst counts |
| Clipped | The prediction cuts into the label by more than 1% of the page on any side (content lost) |
| Count mismatch | Different number of rectangles (a missed or extra split) |
| Accepted | IoU >= 0.95, same count, not clipped: the result could be used without editing |
| Waste | Predicted area minus labelled area, as a fraction of the page (positive keeps extra margin) |

```bash
briss bench --dataset labels --detectors legacy --report bench.md
```

Predictions made by something other than briss (a script, a model run elsewhere, an
earlier version) are scored with a `dir:` detector: `dir:PATH` reads
`PATH/REL.crops.json` for each `REL.pdf` of the dataset.

```bash
python3 spike/tools/layout_propose.py labels/novel.pdf pred/novel.crops.json --keep-headers
```

```bash
briss bench --dataset labels --detectors cv,dir:pred
```

`spike/tools/layout_propose.py` writes such predictions from a rapid_layout model
(PP-DocLayoutV3 by default). To look at labels or predictions, `briss preview PDF
--pages 1-8` draws the current detection on the pages, and
`spike/tools/review_sheets.py PDF OUTDIR NAME=FILE.crops.json ...` draws one or more
crop definitions on contact sheets in different colours.

## Drafting labels from two detectors

The real-scan set was labelled this way, which is much faster than drawing every
rectangle: run two independent detectors (here `cv` and PP-DocLayoutV3), accept the
sheets where they agree (same count, IoU >= 0.9 per rectangle, no edge more than 1.5%
apart), and look at every other sheet, zoomed in on the disputed edges, choosing one
proposal per rectangle or drawing one by hand. Record each choice so the labels can be
rebuilt and audited. The catch is proposer bias: both detectors score better against
such labels than against labels drawn from scratch, so treat the numbers as an upper
bound until a person has reviewed the labels.

## Other label formats

Labels in another form (per-page boxes in CSV, COCO or Pascal VOC boxes, DocLayNet
annotations, pixel boxes on page images) need a small converter into page overrides;
`spike/tools/labels_to_cropdef.py` is an example for the synthetic corpus.
