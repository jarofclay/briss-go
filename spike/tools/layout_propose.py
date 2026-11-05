#!/usr/bin/env python3
"""Propose crops for every page of a PDF with a layout model (rapid_layout,
default PP-DocLayoutV3) and write them as a crop definition (format v1) with
one page override per page: the union of the detected regions, split into a
left and a right page on two-up sheets. Used to draft labels and as a
precomputed detector for `briss bench` (see docs/datasets.md).

usage: layout_propose.py PDF OUT.crops.json [--model pp_doc_layoutv3]
       [--exclude seal] [--keep-headers] [--conf 0.4] [--width 1400]
       [--boxes RAW.json] [--password PW]
"""
import argparse, hashlib, json, os
import pypdfium2 as pdfium
from rapid_layout import RapidLayout

HEADERS = {"header", "footer", "number", "headerimage", "footerimage", "pageheader", "pagefooter", "pagenumber"}

ap = argparse.ArgumentParser(description=__doc__)
ap.add_argument("pdf")
ap.add_argument("out")
ap.add_argument("--model", default="pp_doc_layoutv3")
ap.add_argument("--exclude", default="seal", help="comma-separated classes never part of the crop")
ap.add_argument("--keep-headers", action="store_true", help="keep running headers, footers and page numbers")
ap.add_argument("--conf", type=float, default=0.4)
ap.add_argument("--width", type=int, default=1400, help="render width in pixels")
ap.add_argument("--boxes", default=None, help="also write the raw detections here")
ap.add_argument("--password", default=None)
args = ap.parse_args()

def norm(c): return c.lower().replace("-", "").replace("_", "").replace(" ", "")
excluded = {norm(c) for c in args.exclude.split(",") if c}
if not args.keep_headers:
    excluded |= HEADERS
engine = RapidLayout(model_type=args.model, conf_thresh=args.conf)
doc = pdfium.PdfDocument(args.pdf, password=args.password)

def split(boxes, w, h):
    """Split column of a two-up sheet: the widest run of columns in the middle
    30% that no box covers, or None."""
    if w <= 1.15 * h or not boxes:
        return None
    cover = [False] * w
    for x0, _, x1, _ in boxes:
        for x in range(max(0, int(x0)), min(w, int(x1) + 1)):
            cover[x] = True
    lo, hi = int(0.35 * w), int(0.65 * w)
    best, run = None, 0
    for x in range(lo, hi + 1):
        if x < hi and not cover[x]:
            run += 1
            continue
        if run >= max(3, w // 200) and (best is None or run > best[1]):
            best = (x - run // 2, run)
        run = 0
    return best[0] if best else None

overrides, raw = {}, {}
for i in range(len(doc)):
    page = doc[i]
    pw, ph = page.get_size()
    img = page.render(scale=args.width / pw).to_numpy()
    h, w = img.shape[:2]
    res = engine(img)
    dets = [(list(map(float, b)), c, float(s)) for b, c, s in zip(res.boxes, res.class_names, res.scores)] if res.boxes is not None else []
    raw[str(i + 1)] = {"size": [w, h], "boxes": [{"box": b, "class": c, "score": round(s, 3)} for b, c, s in dets]}
    keep = [b for b, c, _ in dets if norm(c) not in excluded]
    s = split(keep, w, h)
    groups = [keep] if s is None else [[b for b in keep if (b[0] + b[2]) / 2 < s], [b for b in keep if (b[0] + b[2]) / 2 >= s]]
    rects = []
    for g in groups:
        if not g:
            continue
        x0, y0 = max(0, min(b[0] for b in g)), max(0, min(b[1] for b in g))
        x1, y1 = min(w, max(b[2] for b in g)), min(h, max(b[3] for b in g))
        rects.append({"l": round(x0 / w, 6), "b": round(1 - y1 / h, 6), "r": round(1 - x1 / w, 6), "t": round(y0 / h, 6)})
    overrides[str(i + 1)] = rects  # empty list: nothing found, keep the page
sha = hashlib.sha256(open(args.pdf, "rb").read()).hexdigest()
d = {"version": 1, "source": {"path": os.path.basename(args.pdf), "sha256": sha, "pages": len(doc)},
     "options": {"keep_headers": args.keep_headers, "detector": f"{args.model} (layout_propose.py)"},
     "clusters": [], "page_overrides": overrides}
json.dump(d, open(args.out, "w"), indent=1)
if args.boxes:
    json.dump(raw, open(args.boxes, "w"))
print(f"wrote {args.out}: {len(doc)} pages")
