#!/usr/bin/env python3
"""Generate the spike test corpus with exact ground-truth crop boxes.

Outputs (in the directory given as argv[1]):
  book300.pdf      300-page born-digital book: running headers, page numbers, figures,
                   a two-column chapter, margin notes, 4 landscape pages, outlines,
                   named destinations, metadata.
  scan_single.pdf  120 scanned-looking pages (skew, offset, noise, paper tint, gutter shadow).
  scan_twoup.pdf   60 landscape sheets with two book pages each (two-up scan).
  labels.json      per-page ground truth in PDF points (origin bottom-left) and briss ratios.

Deterministic (seed 42). Requires reportlab, pypdfium2, pillow, numpy.
"""
import io, json, math, os, random, sys

import numpy as np
from PIL import Image, ImageFilter
from reportlab.lib.pagesizes import letter, landscape
from reportlab.pdfgen import canvas
from reportlab.lib.utils import ImageReader

random.seed(42)
np.random.seed(42)
OUT = sys.argv[1] if len(sys.argv) > 1 else "."
os.makedirs(OUT, exist_ok=True)

W, H = letter  # 612 x 792
VOCAB = ("the of and to in a is that for it as was with be by on not he this are or his from at which but "
         "have an they you were her she all their there been one if more when will would who so no out up "
         "into about than them can only other new some could time these two may then do first any my now "
         "such like our over man even most made after also did many before must through back years where "
         "much your way well down should because each just those people how too little state good very make "
         "world still own see men work long get here between both life being under never day same another "
         "know while last might us great old year off come since against go came right used take three").split()

def words(n):
    return " ".join(random.choice(VOCAB) for _ in range(n))

def paragraph():
    return (words(random.randint(40, 110)) + ".").capitalize()

labels = {"book300": {}, "scan_single": {}, "scan_twoup": {}}

def ratios(box, pw, ph):
    """briss ratios {l,b,r,t} as fractions from each edge; box = (x0,y0,x1,y1) points, origin bottom-left."""
    x0, y0, x1, y1 = box
    return {"l": x0 / pw, "b": y0 / ph, "r": 1 - x1 / pw, "t": 1 - y1 / ph}

# ---------------------------------------------------------------- book300
def gen_book(path):
    c = canvas.Canvas(path, pagesize=letter)
    c.setTitle("Spike Corpus: A Synthetic Book"); c.setAuthor("briss-go spike"); c.setSubject("test corpus")
    title = "A Synthetic Book"
    chapters = [f"Chapter {i}: {words(3).title()}" for i in range(1, 13)]
    page = 0
    body_font, body_size, lead = "Times-Roman", 11, 14
    inner, outer, top, bottom = 90, 72, 72, 72  # margins in points
    header_y = H - 46
    footer_y = 36

    def start_page(is_landscape=False):
        nonlocal page
        page += 1
        if is_landscape:
            c.setPageSize(landscape(letter))
        else:
            c.setPageSize(letter)
        return page

    def frame(p):
        """body frame for page p (odd pages: inner margin on left)."""
        if p % 2 == 1:
            return inner, bottom, W - outer, H - top
        return outer, bottom, W - inner, H - top

    def header_footer(p, chapter):
        x0, _, x1, _ = frame(p)
        c.setFont("Times-Italic", 9)
        if p % 2 == 0:
            c.drawString(x0, header_y, title)
        else:
            c.drawRightString(x1, header_y, chapter)
        c.setLineWidth(0.4); c.line(x0, header_y - 4, x1, header_y - 4)
        c.setFont("Times-Roman", 9)
        if p % 2 == 0:
            c.drawString(x0, footer_y, str(p))
        else:
            c.drawRightString(x1, footer_y, str(p))
        hb = (x0, header_y - 4, x1, header_y + 7)
        fb = (x0, footer_y - 2, x1, footer_y + 7)
        return hb, fb

    def record(p, body, header, footer, extra=None, pw=W, ph=H):
        labels["book300"][str(p)] = {
            "size": [pw, ph], "body": list(body), "header": list(header), "footer": list(footer),
            "extra": extra or {}, "ratios_body": ratios(body, pw, ph)}

    def text_lines(x0, y_top, x1, lines, font=body_font, size=body_size, leading=lead):
        c.setFont(font, size)
        t = c.beginText(x0, y_top - size)
        t.setLeading(leading)
        for ln in lines:
            t.textLine(ln)
        c.drawText(t)
        n = len(lines)
        while n > 0 and not lines[n - 1].strip():  # trailing blank spacer lines carry no ink
            n -= 1
        return y_top - size - leading * (max(n, 1) - 1) - 3  # bottom of last inked line (approx descender)

    def wrap(text, width_pt, size=body_size, font=body_font):
        from reportlab.pdfbase.pdfmetrics import stringWidth
        out, cur = [], []
        for w in text.split():
            trial = " ".join(cur + [w])
            if stringWidth(trial, font, size) > width_pt and cur:
                out.append(" ".join(cur)); cur = [w]
            else:
                cur.append(w)
        if cur: out.append(" ".join(cur))
        return out

    # title page
    p = start_page()
    c.setFont("Times-Bold", 28); c.drawCentredString(W / 2, H - 300, title)
    c.setFont("Times-Roman", 14); c.drawCentredString(W / 2, H - 340, "briss-go spike corpus")
    record(p, (W / 2 - 110, H - 346, W / 2 + 110, H - 272), (0, 0, 0, 0), (0, 0, 0, 0))
    c.bookmarkPage("title"); c.addOutlineEntry("Title page", "title", level=0)
    c.showPage()
    # contents page
    p = start_page()
    hb, fb = header_footer(p, "Contents")
    x0, y0, x1, y1 = frame(p)
    lines = ["Contents", ""] + chapters
    yb = text_lines(x0, y1, x1, lines)
    record(p, (x0, yb, x1 - 120, y1), hb, fb)
    c.bookmarkPage("contents"); c.addOutlineEntry("Contents", "contents", level=0)
    c.showPage()

    pages_per_chapter = 24
    for ci, chapter in enumerate(chapters):
        for k in range(pages_per_chapter):
            is_land = (ci == 4 and k in (10, 11)) or (ci == 10 and k in (6, 7))  # 4 landscape pages
            if is_land:
                p = start_page(True)
                lw, lh = landscape(letter)
                c.setFont("Times-Italic", 9); c.drawRightString(lw - 72, lh - 40, chapter)
                c.setFont("Times-Roman", 9); c.drawCentredString(lw / 2, 30, str(p))
                fx0, fy0, fx1, fy1 = 100, 110, lw - 100, lh - 110
                c.setFillGray(0.75); c.rect(fx0, fy0, fx1 - fx0, fy1 - fy0, fill=1, stroke=0); c.setFillGray(0)
                c.setFont("Times-Roman", 10); c.drawCentredString(lw / 2, fy0 - 16, f"Figure {ci+1}.{k}: {words(5)}")
                record(p, (fx0, fy0 - 20, fx1, fy1), (lw - 300, lh - 42, lw - 72, lh - 31), (lw / 2 - 10, 28, lw / 2 + 10, 37),
                       {"kind": "landscape-figure"}, lw, lh)
                c.showPage(); continue
            p = start_page()
            hb, fb = header_footer(p, chapter)
            x0, y0, x1, y1 = frame(p)
            extra = {}
            y = y1
            if k == 0:
                c.bookmarkPage(f"ch{ci+1}"); c.addOutlineEntry(chapter, f"ch{ci+1}", level=0)
                c.setFont("Times-Bold", 20); c.drawString(x0, y - 60, chapter); y -= 90
            if ci == 6:  # two-column chapter
                colgap = 18; cw = (x1 - x0 - colgap) / 2
                cols = []
                for cx in (x0, x0 + cw + colgap):
                    lines = []
                    while len(lines) < int((y - y0) / lead) - 1:
                        lines += wrap(paragraph(), cw, 10, body_font) + [""]
                    lines = lines[: int((y - y0) / lead) - 1]
                    yb = text_lines(cx, y, cx + cw, lines, size=10, leading=lead)
                    cols.append((cx, yb, cx + cw, y))
                extra = {"kind": "two-column", "columns": [list(b) for b in cols]}
                body = (x0, min(b[1] for b in cols), x1, y1)
            else:
                lines = []
                target = int((y - y0) / lead) - 1
                fig = (k % 7 == 3)
                if fig:
                    target -= 12
                while len(lines) < target:
                    lines += wrap(paragraph(), x1 - x0) + [""]
                lines = lines[:target]
                yb = text_lines(x0, y, x1, lines)
                body_bottom = yb
                if fig:
                    fh = 11 * lead; fy1 = yb - 10; fy0 = fy1 - fh + 20
                    c.setFillGray(0.8); c.rect(x0 + 40, fy0, x1 - x0 - 80, fh - 20, fill=1, stroke=0); c.setFillGray(0)
                    c.setFont("Times-Roman", 9); c.drawCentredString((x0 + x1) / 2, fy0 - 12, f"Figure {ci+1}.{k}: {words(4)}")
                    body_bottom = fy0 - 14
                    extra = {"kind": "figure"}
                if ci == 8 and k % 2 == 0:  # margin notes chapter
                    mx0 = x1 + 10 if p % 2 == 1 else x0 - 60
                    mx1 = mx0 + 50
                    mlines = wrap(words(12), 50, 7, "Times-Italic")
                    c.setFont("Times-Italic", 7)
                    t = c.beginText(mx0, y1 - 120); t.setLeading(9)
                    for ln in mlines: t.textLine(ln)
                    c.drawText(t)
                    mb = (mx0, y1 - 120 - 9 * len(mlines), mx1, y1 - 112)
                    extra = {"kind": "margin-note", "margin_note": list(mb)}
                body = (x0, body_bottom, x1, y1 if k != 0 else y1)
                if k == 0:
                    body = (x0, body_bottom, x1, y1 - 40)
            if k == 5:
                c.bookmarkPage(f"ch{ci+1}s1"); c.addOutlineEntry("Section " + words(2).title(), f"ch{ci+1}s1", level=1)
            record(p, body, hb, fb, extra)
            c.showPage()
    c.save()
    return page

n = gen_book(os.path.join(OUT, "book300.pdf"))
print("book300.pdf pages:", n)

# ---------------------------------------------------------------- scans
import pypdfium2 as pdfium
pdf = pdfium.PdfDocument(os.path.join(OUT, "book300.pdf"))
DPI = 150
S = DPI / 72.0

def render_gray(i):
    pg = pdf[i]
    bm = pg.render(scale=S, grayscale=True)
    return bm.to_pil().convert("L")

def scanify(img, angle, dx, dy, tint, gutter_side):
    w, h = img.size
    canvas_img = Image.new("L", (w, h), tint)
    rot = img.rotate(angle, resample=Image.BICUBIC, expand=False, fillcolor=255)
    canvas_img.paste(rot, (dx, dy))
    arr = np.asarray(canvas_img).astype(np.float32)
    arr = np.minimum(arr, tint + np.random.normal(0, 1, arr.shape))  # paper tint cap
    arr += np.random.normal(0, 4, arr.shape)
    # specks
    for _ in range(random.randint(20, 60)):
        x, y = random.randrange(w), random.randrange(h)
        arr[max(0, y - 1):y + 2, max(0, x - 1):x + 2] -= random.randint(60, 160)
    # gutter shadow on one side
    if gutter_side:
        band = int(w * 0.035)
        grad = np.linspace(90, 0, band)[None, :]
        if gutter_side == "left":
            arr[:, :band] -= grad
        else:
            arr[:, w - band:] -= grad[:, ::-1]
    arr = np.clip(arr, 0, 255).astype(np.uint8)
    return Image.fromarray(arr)

def transform_box(box, pw, ph, angle_deg, dx_pt, dy_pt):
    """axis-aligned bbox of a page-space rectangle after rotation about page center and translation."""
    cx, cy = pw / 2, ph / 2
    a = math.radians(angle_deg)
    pts = [(box[0], box[1]), (box[2], box[1]), (box[2], box[3]), (box[0], box[3])]
    out = []
    for x, y in pts:
        rx = cx + (x - cx) * math.cos(a) - (y - cy) * math.sin(a)
        ry = cy + (x - cx) * math.sin(a) + (y - cy) * math.cos(a)
        out.append((rx + dx_pt, ry + dy_pt))
    xs = [p[0] for p in out]; ys = [p[1] for p in out]
    return (max(0, min(xs)), max(0, min(ys)), min(pw, max(xs)), min(ph, max(ys)))

def jpeg_reader(img, q=55):
    buf = io.BytesIO(); img.save(buf, format="JPEG", quality=q); buf.seek(0)
    return ImageReader(buf)

# scan_single: book pages 20..139 (0-based indices), portrait only
c = canvas.Canvas(os.path.join(OUT, "scan_single.pdf"), pagesize=letter)
c.setTitle("Spike Corpus: single-page scan")
out_p = 0
for i in range(20, 140):
    src = labels["book300"].get(str(i + 1))
    if not src or src["size"][0] > src["size"][1]:
        continue
    img = render_gray(i)
    angle = random.uniform(-1.2, 1.2)
    dx, dy = random.randint(-25, 25), random.randint(-25, 25)
    tint = random.randint(226, 242)
    gutter = random.choice([None, "left", "right"])
    scan = scanify(img, angle, dx, dy, tint, gutter)
    c.setPageSize(letter)
    c.drawImage(jpeg_reader(scan), 0, 0, width=W, height=H)
    out_p += 1
    # PIL rotate is counter-clockwise for positive angle in image space (y down) => clockwise in PDF space
    gt = transform_box(src["body"], W, H, -angle, dx / S, -dy / S)
    labels["scan_single"][str(out_p)] = {"size": [W, H], "source_page": i + 1, "body": list(gt),
                                         "ratios_body": ratios(gt, W, H), "angle": angle, "dx": dx, "dy": dy}
    c.showPage()
c.save()
print("scan_single.pdf pages:", out_p)

# scan_twoup: 60 sheets of pages (140+2k, 141+2k)
LW, LH = landscape(letter)
c = canvas.Canvas(os.path.join(OUT, "scan_twoup.pdf"), pagesize=landscape(letter))
c.setTitle("Spike Corpus: two-up scan")
out_p = 0
half = W * (LW / (2 * W))  # scale each page to half the sheet width
scale = (LW / 2) / W       # 0.6471 ; sheet height 612 vs page height 792*scale=512.5 -> fits with margins
for k in range(60):
    i0 = 140 + 2 * k
    srcs = [labels["book300"].get(str(i0 + 1)), labels["book300"].get(str(i0 + 2))]
    if not all(srcs) or any(s["size"][0] > s["size"][1] for s in srcs):
        continue
    sheet = Image.new("L", (int(LW * S), int(LH * S)), 230)
    boxes = []
    for j, i in enumerate((i0, i0 + 1)):
        img = render_gray(i)
        angle = random.uniform(-0.8, 0.8)
        pg = scanify(img, angle, random.randint(-10, 10), random.randint(-10, 10), 232, None)
        pw_px = int(W * S * scale); ph_px = int(H * S * scale)
        pg = pg.resize((pw_px, ph_px), Image.LANCZOS)
        ox = int(j * LW * S / 2) + random.randint(-6, 6)
        oy = int((LH * S - ph_px) / 2) + random.randint(-6, 6)
        sheet.paste(pg, (ox, oy))
        b = srcs[j]["body"]
        # page-space box -> sheet space (points, origin bottom-left)
        bx0 = ox / S + b[0] * scale; bx1 = ox / S + b[2] * scale
        by1 = LH - oy / S - (H - b[3]) * scale; by0 = LH - oy / S - (H - b[1]) * scale
        pad = 6 * scale  # skew tolerance
        boxes.append((bx0 - pad, by0 - pad, bx1 + pad, by1 + pad))
    arr = np.asarray(sheet).astype(np.float32)
    mid = arr.shape[1] // 2
    arr[:, mid - 12: mid + 12] -= np.linspace(0, 110, 24)[None, :] * np.linspace(1, 0.2, 24)[None, ::-1]
    arr += np.random.normal(0, 3, arr.shape)
    sheet = Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8))
    c.setPageSize(landscape(letter))
    c.drawImage(jpeg_reader(sheet), 0, 0, width=LW, height=LH)
    out_p += 1
    labels["scan_twoup"][str(out_p)] = {"size": [LW, LH], "source_pages": [i0 + 1, i0 + 2],
                                        "boxes": [list(b) for b in boxes],
                                        "ratios": [ratios(b, LW, LH) for b in boxes]}
    c.showPage()
c.save()
print("scan_twoup.pdf pages:", out_p)

with open(os.path.join(OUT, "labels.json"), "w") as f:
    json.dump(labels, f, indent=1)
print("labels.json written")
