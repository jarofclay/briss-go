#!/usr/bin/env python3
"""Convert the synthetic corpus labels (labels.json from gen_corpus.py) into
bench labels: one NAME.crops.json crop definition (format v1) next to each PDF,
with a page override per page holding the ground-truth body box (two boxes,
left then right, for the two-up scans).

usage: labels_to_cropdef.py <testdata dir>
"""
import hashlib, json, os, sys

td = sys.argv[1]
labels = json.load(open(os.path.join(td, "labels.json")))

def ratios(box, w, h):
    x0, y0, x1, y1 = box
    return {"l": round(x0 / w, 6), "b": round(y0 / h, 6), "r": round(1 - x1 / w, 6), "t": round(1 - y1 / h, 6)}

for name, pages in labels.items():
    pdf = os.path.join(td, name + ".pdf")
    sha = hashlib.sha256(open(pdf, "rb").read()).hexdigest()
    overrides = {}
    for p, lb in pages.items():
        w, h = lb["size"]
        boxes = lb.get("boxes") or [lb["body"]]
        overrides[p] = [ratios(b, w, h) for b in boxes]
    d = {"version": 1, "source": {"path": name + ".pdf", "sha256": sha, "pages": len(pages)},
         "options": {"detector": "ground truth (gen_corpus.py)"}, "clusters": [], "page_overrides": overrides}
    out = os.path.join(td, name + ".crops.json")
    json.dump(d, open(out, "w"), indent=1)
    print("wrote", out, len(overrides), "pages")
