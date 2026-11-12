#!/usr/bin/env bash
# Rebuilds the demo images in site/assets/demo from a synthetic scan, using briss itself.
# Needs: node with playwright and @fontsource/eb-garamond, python3 with Pillow, pdftoppm,
# and a built ./briss in the repository root.   Usage: tools/site-demo/build.sh WORKDIR
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"; root="$(cd "$here/../.." && pwd)"
work="${1:?work directory}"; briss="${BRISS:-$root/briss}"; dest="$root/site/assets/demo"
mkdir -p "$work/sheets" "$work/pages" "$work/preview" "$dest"
(cd "$work" && node "$here/spreads.mjs" "$work/sheets")
python3 - "$work" <<'PY'
import sys
from PIL import Image
w = sys.argv[1]
ims = [Image.open(f"{w}/sheets/sheet-{i}.png").convert("RGB") for i in (1, 2, 3)]
ims[2] = ims[2].rotate(90, expand=True)   # the third sheet was laid on the glass sideways
ims[0].save(f"{w}/A Reader's Companion.pdf", save_all=True, append_images=ims[1:], resolution=150, quality=88)
PY
cd "$work"
"$briss" crop "A Reader's Companion.pdf" --straighten --normalize --keep-headers --margin 6pt --save-crops demo.crops.json | tee crop.txt
"$briss" detect "A Reader's Companion.pdf" --keep-headers --json > detect.json
"$briss" preview "A Reader's Companion.pdf" --keep-headers --out preview | tee preview.txt
pdftoppm -r 110 -png "A Reader's Companion_cropped.pdf" pages/page
pdftoppm -r 190 -png "A Reader's Companion_cropped.pdf" pages/hi
python3 - "$work" "$dest" <<'PY'
import sys, glob
from PIL import Image
w, d = sys.argv[1:3]
def save(im, name, width, q=80):
    im = im.convert("RGB")
    if im.width > width: im = im.resize((width, round(width * im.height / im.width)), Image.LANCZOS)
    im.save(f"{d}/{name}.webp", quality=q, method=6)
for i in (1, 2, 3):
    im = Image.open(f"{w}/sheets/sheet-{i}.png")
    if i == 3: im = im.rotate(90, expand=True)
    save(im, f"sheet-{i}", 1800 if i == 1 else 900)
    save(Image.open(sorted(glob.glob(f"{w}/preview/*.png"))[i - 1]), f"preview-{i}", 900, 82)
for i in range(1, 7):
    save(Image.open(f"{w}/pages/hi-{i}.png"), f"page-{i}", 900, 82)
PY
cp demo.crops.json "$dest/demo.crops.json"
