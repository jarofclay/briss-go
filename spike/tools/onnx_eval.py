#!/usr/bin/env python3
"""Spike 5a: CPU latency and crop accuracy of the CDLA layout model (PicoDet, bundled
with the rapid_layout wheel) on corpus pages. Crop = union of boxes whose class is
not Header/Footer. Compares with labels.json ground truth.

usage: onnx_eval.py <testdata dir> [pages per file] [--model TYPE] [--path file.onnx] [--exclude a,b]
"""
import argparse, json, os, sys, time, statistics
import numpy as np
import pypdfium2 as pdfium
import onnxruntime as ort
from rapid_layout import RapidLayout

# classes that must stay outside the crop, per rapid_layout model type (matched
# case-insensitively with '-', '_' and spaces removed)
DEFAULT_EXCLUDE = {
    "pp_layout_cdla": "header,footer",
    "pp_layout_publaynet": "",
    "pp_doc_layoutv2": "header,footer,number,header_image,footer_image",
    "pp_doc_layoutv3": "header,footer,number,header_image,footer_image",
    "doclayout_docstructbench": "abandon",
    "doclayout_d4la": "page-header,page-footer,page-number,header,footer,number",
    "doclayout_docsynth": "page-header,page-footer",
}

ap = argparse.ArgumentParser(description=__doc__)
ap.add_argument("testdata")
ap.add_argument("pages", nargs="?", type=int, default=40)
ap.add_argument("--model", default="pp_layout_cdla", help="rapid_layout model type (downloads on first use)")
ap.add_argument("--path", default=None, help="local .onnx file to use instead of the download")
ap.add_argument("--exclude", default=None, help="comma-separated class names to exclude (default per model)")
ap.add_argument("--conf", type=float, default=0.4)
args = ap.parse_args()

TD, N = args.testdata, args.pages
labels = json.load(open(os.path.join(TD, "labels.json")))
kw = {"model_type": args.model, "conf_thresh": args.conf}
if args.path:
    kw["model_dir_or_path"] = args.path
engine = RapidLayout(**kw)
def norm(c): return c.lower().replace("-", "").replace("_", "").replace(" ", "")
EXCLUDED = {norm(c) for c in (args.exclude if args.exclude is not None else DEFAULT_EXCLUDE.get(args.model, "")).split(",") if c}
print(f"model={args.model} path={args.path or 'auto'} excluded classes={sorted(EXCLUDED)}")

def iou(a, b):
    ix0, iy0, ix1, iy1 = max(a[0], b[0]), max(a[1], b[1]), min(a[2], b[2]), min(a[3], b[3])
    inter = max(0, ix1 - ix0) * max(0, iy1 - iy0)
    ua = (a[2] - a[0]) * (a[3] - a[1]) + (b[2] - b[0]) * (b[3] - b[1]) - inter
    return inter / ua if ua > 0 else 0

def snap(img_gray, box_px, blockers_px, max_pt_px, thr=200, min_ink=3, max_gap_px=8):
    """Grow box outward on each side while ink rows/cols continue within max_pt_px,
    without entering any blocker box (detected header/footer). Pixel coords, y down."""
    h, w = img_gray.shape
    x0, y0, x1, y1 = [int(round(v)) for v in box_px]
    ink = img_gray < thr
    def blocked(side, pos):
        for bx0, by0, bx1, by1 in blockers_px:
            if side in "TB" and bx0 < x1 and bx1 > x0 and by0 <= pos <= by1: return True
            if side in "LR" and by0 < y1 and by1 > y0 and bx0 <= pos <= bx1: return True
        return False
    def grow(side):
        nonlocal x0, y0, x1, y1
        gap, last = 0, None
        for step in range(1, max_pt_px):
            if side == "B": pos = y1 + step; ok = pos < h and not blocked("B", pos); line = ink[pos, x0:x1] if ok else None
            if side == "T": pos = y0 - step; ok = pos >= 0 and not blocked("T", pos); line = ink[pos, x0:x1] if ok else None
            if side == "R": pos = x1 + step; ok = pos < w and not blocked("R", pos); line = ink[y0:y1, pos] if ok else None
            if side == "L": pos = x0 - step; ok = pos >= 0 and not blocked("L", pos); line = ink[y0:y1, pos] if ok else None
            if not ok: break
            if line.sum() >= min_ink: last, gap = pos, 0
            else:
                gap += 1
                if gap > max_gap_px: break
        if last is not None:
            if side == "B": y1 = last + 1
            if side == "T": y0 = last
            if side == "R": x1 = last + 1
            if side == "L": x0 = last
    for side in "BTRL": grow(side)
    return (x0, y0, x1, y1)

def run(pdfname, key, idxs, excluded=None, pad_pt=0.0, snap_pt=0.0):
    excluded = EXCLUDED if excluded is None else {norm(c) for c in excluded}
    pdf = pdfium.PdfDocument(os.path.join(TD, pdfname))
    lat, ious, clipped, acc, nodet = [], [], 0, 0, 0
    for i in idxs:
        lb = labels[key].get(str(i + 1))
        if not lb or "body" not in lb:
            continue
        pw, ph = lb["size"]
        scale = 1024 / ph
        pg = pdf[i]
        img = pg.render(scale=scale).to_numpy()  # RGB
        t = time.perf_counter()
        res = engine(img)
        lat.append(time.perf_counter() - t)
        keep = [b for b, c in zip(res.boxes, res.class_names) if norm(c) not in excluded]
        if not keep:
            nodet += 1
            continue
        x0 = min(b[0] for b in keep); y0 = min(b[1] for b in keep)
        x1 = max(b[2] for b in keep); y1 = max(b[3] for b in keep)
        if snap_pt > 0:
            gray = img.mean(axis=2)
            blockers = [b for b, c in zip(res.boxes, res.class_names) if norm(c) in excluded]
            x0, y0, x1, y1 = snap(gray, (x0, y0, x1, y1), blockers, int(snap_pt * scale))
        pred = (x0 / scale - pad_pt, ph - y1 / scale - pad_pt, x1 / scale + pad_pt, ph - y0 / scale + pad_pt)  # to PDF points, origin bottom-left
        gt = lb["body"]
        v = iou(pred, gt); ious.append(v)
        tol = 0.01 * ph
        clip = pred[0] > gt[0] + tol or pred[1] > gt[1] + tol or pred[2] < gt[2] - tol or pred[3] < gt[3] - tol
        clipped += clip
        if clip and snap_pt > 0:
            side = [n for n, c in zip("LBRT", (pred[0] > gt[0] + tol, pred[1] > gt[1] + tol, pred[2] < gt[2] - tol, pred[3] < gt[3] - tol)) if c]
            print(f"   clipped p{i+1} kind={lb.get('extra', {}).get('kind', '-')} sides={''.join(side)} gt={[round(v) for v in gt]} pred={[round(v) for v in pred]}")
        acc += (v >= 0.95 and not clip)
    n = len(ious)
    print(f"{pdfname} pad={pad_pt}pt snap={snap_pt}pt: pages={len(lat)} latency p50={statistics.median(lat)*1000:.0f}ms p95={np.percentile(lat,95)*1000:.0f}ms "
          f"mean IoU={statistics.mean(ious) if ious else 0:.3f} clipped={clipped}/{n} accept(IoU>=.95,no clip)={acc}/{n} no_detection={nodet}")
    return res

# raw model latency: session.run only, on the model the engine loaded
try:
    s = engine.session.session
    inp = s.get_inputs()[0]
    shape = [d if isinstance(d, int) else (1 if i == 0 else 1024) for i, d in enumerate(inp.shape)]
    x = np.random.rand(*shape).astype(np.float32)
    feeds = {inp.name: x}
    for extra in s.get_inputs()[1:]:  # e.g. PP models with a scale_factor input
        feeds[extra.name] = np.ones([d if isinstance(d, int) else 1 for d in extra.shape], dtype=np.float32)
    for _ in range(3): s.run(None, feeds)
    ts = []
    for _ in range(20):
        t = time.perf_counter(); s.run(None, feeds); ts.append(time.perf_counter() - t)
    print(f"raw session.run {'x'.join(map(str, shape))}: p50={statistics.median(ts)*1000:.0f}ms min={min(ts)*1000:.0f}ms threads={os.cpu_count()}")
except Exception as e:  # engine wrappers differ between rapid_layout versions
    print("raw session timing skipped:", type(e).__name__, e)

for pad, sn in ((0.0, 0.0), (2.0, 24.0), (2.0, 40.0)):
    run("book300.pdf", "book300", list(range(2, 2 + 6 * N, 6)), pad_pt=pad, snap_pt=sn)
    run("scan_single.pdf", "scan_single", list(range(0, min(118, 3 * N), 3)), pad_pt=pad, snap_pt=sn)
