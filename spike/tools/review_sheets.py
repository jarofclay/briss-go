#!/usr/bin/env python3
"""Draw one or more crop definitions on the pages of a PDF, a grid of pages per
image, for reviewing detector output and labels by eye.

usage: review_sheets.py PDF OUTDIR NAME=CROPS.json [NAME=CROPS.json ...]
         [--pages 1-12] [--cols 3] [--width 520] [--password PW]

Each definition gets its own colour (red, blue, green, orange, ...); rectangles
are numbered in output order. Pages are labelled "p<number>".
"""
import argparse, json, os
import pypdfium2 as pdfium
from PIL import Image, ImageDraw

COLOURS = [(220, 30, 30), (30, 110, 230), (20, 160, 60), (230, 140, 0), (150, 40, 200)]

def page_rects(defn, page):
    ov = defn.get("page_overrides") or {}
    if str(page) in ov:
        return ov[str(page)]
    for c in defn.get("clusters") or []:
        if page in (c.get("pages") or []):
            return c.get("rects") or []
    return []

def parse_pages(s, n):
    if not s:
        return list(range(1, n + 1))
    out = []
    for tok in s.replace(",", ";").split(";"):
        a, _, b = tok.partition("-")
        out += list(range(int(a), int(b or a) + 1))
    return [p for p in out if 1 <= p <= n]

ap = argparse.ArgumentParser()
ap.add_argument("pdf"); ap.add_argument("outdir"); ap.add_argument("defs", nargs="+")
ap.add_argument("--pages", default=""); ap.add_argument("--cols", type=int, default=3)
ap.add_argument("--rows", type=int, default=3); ap.add_argument("--width", type=int, default=520)
ap.add_argument("--password", default=None)
a = ap.parse_args()
defs = []
for d in a.defs:
    name, _, path = d.partition("=")
    defs.append((name, json.load(open(path))))
pdf = pdfium.PdfDocument(a.pdf, password=a.password)
pages = parse_pages(a.pages, len(pdf))
os.makedirs(a.outdir, exist_ok=True)
base = os.path.splitext(os.path.basename(a.pdf))[0]
tiles = []
for p in pages:
    pg = pdf[p - 1]
    w, h = pg.get_size()
    im = pg.render(scale=a.width / w).to_pil().convert("RGB")
    W, H = im.size
    dr = ImageDraw.Draw(im)
    for k, (name, d) in enumerate(defs):
        col = COLOURS[k % len(COLOURS)]
        for i, r in enumerate(page_rects(d, p)):
            x0, y0 = r["l"] * W, r["t"] * H
            x1, y1 = (1 - r["r"]) * W, (1 - r["b"]) * H
            dr.rectangle([x0 + k, y0 + k, x1 - k, y1 - k], outline=col, width=2)
            dr.text((x0 + 4 + 12 * k, y0 + 3), str(i + 1), fill=col)
    dr.rectangle([0, 0, 40, 12], fill=(255, 255, 255)); dr.text((2, 0), f"p{p}", fill=(0, 0, 0))
    tiles.append(im)
per = a.cols * a.rows
written = []
for s in range(0, len(tiles), per):
    chunk = tiles[s:s + per]
    tw = max(t.width for t in chunk); th = max(t.height for t in chunk)
    rows = (len(chunk) + a.cols - 1) // a.cols
    sheet = Image.new("RGB", (a.cols * (tw + 6), rows * (th + 6)), (70, 70, 70))
    for i, t in enumerate(chunk):
        sheet.paste(t, ((i % a.cols) * (tw + 6), (i // a.cols) * (th + 6)))
    out = os.path.join(a.outdir, f"{base}_review{s // per + 1:02d}.png")
    sheet.save(out); written.append(out)
print("\n".join(written))
print("legend:", ", ".join(f"{n}={COLOURS[k % len(COLOURS)]}" for k, (n, _) in enumerate(defs)))
