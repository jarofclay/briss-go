#!/usr/bin/env python3
"""Spike 3 verifier: compares a cropped PDF against its source.
Checks page count, boxes, outline and named-destination targets (by page text),
metadata, and renders two output pages to PNG for a visual check.
usage: verify_crop.py src.pdf out.pdf labels.json pngdir
"""
import json, os, sys
from pypdf import PdfReader
import pypdfium2 as pdfium

src, out, labels_path, pngdir = sys.argv[1:5]
rs, ro = PdfReader(src), PdfReader(out)
labels = json.load(open(labels_path))["book300"]
print(f"pages: src={len(rs.pages)} out={len(ro.pages)}")

def sig(page):
    return (page.extract_text() or "")[:80].strip()

# expected mapping src page -> first out page (clones follow two-column pages)
exp = {}
o = 0
for p in range(1, len(rs.pages) + 1):
    exp[p] = o
    lb = labels.get(str(p), {})
    o += 2 if lb.get("extra", {}).get("kind") == "two-column" else 1
assert o == len(ro.pages), f"expected {o} output pages, got {len(ro.pages)}"

bad = 0
for p in range(1, len(rs.pages) + 1):
    s = sig(rs.pages[p - 1]); t = sig(ro.pages[exp[p]])
    if s != t:
        bad += 1
        if bad < 4: print("  text mismatch at src page", p, "->", exp[p], repr(s[:30]), repr(t[:30]))
    pg = ro.pages[exp[p]]
    if [float(v) for v in pg.mediabox] != [float(v) for v in pg.cropbox]:
        bad += 1; print("  media/crop differ on out page", exp[p])
    lb = labels.get(str(p))
    if lb and lb.get("extra", {}).get("kind") == "two-column":
        a, b = ro.pages[exp[p]], ro.pages[exp[p] + 1]
        if sig(a) != sig(b): bad += 1; print("  clone text differs at", exp[p])
        if float(a.cropbox.right) > float(b.cropbox.left) + 1: bad += 1; print("  columns overlap at", exp[p])
print("page text/box checks: mismatches =", bad)

def outline_targets(reader):
    res = []
    def walk(items, depth=0):
        for it in items:
            if isinstance(it, list): walk(it, depth + 1)
            else: res.append((depth, it.title, reader.get_destination_page_number(it)))
    walk(reader.outline)
    return res

so, oo = outline_targets(rs), outline_targets(ro)
print(f"outlines: src={len(so)} out={len(oo)}")
obad = 0
for (d1, t1, p1), (d2, t2, p2) in zip(so, oo):
    if t1 != t2 or p2 != exp[p1 + 1]:
        obad += 1
        if obad < 4: print("  outline mismatch:", t1, p1, "->", t2, p2, "expected", exp[p1 + 1])
print("outline target mismatches =", obad)

ns, no = rs.named_destinations, ro.named_destinations
nbad = 0
for name, d in ns.items():
    p1 = rs.get_destination_page_number(d)
    if name not in no: nbad += 1; continue
    p2 = ro.get_destination_page_number(no[name])
    if p2 != exp[p1 + 1]: nbad += 1
print(f"named destinations: src={len(ns)} out={len(no)} mismatches={nbad}")
print("metadata:", dict(ro.metadata or {}))

os.makedirs(pngdir, exist_ok=True)
doc = pdfium.PdfDocument(out)
for i in (exp[10], exp[150], exp[150] + 1):
    pg = doc[i]
    print(f"  out page {i}: size={pg.get_size()} bbox={pg.get_bbox()}")
    pg.render(scale=1).to_pil().save(os.path.join(pngdir, f"out_page{i:03d}.png"))
print("rendered", pngdir)
