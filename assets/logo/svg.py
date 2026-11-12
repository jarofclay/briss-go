#!/usr/bin/env python3
"""Write the Briss logo as SVG files.

The mark is a faint page with the part that survives the crop lifted out in
front of it, framed by two blue crop corners, on the app's dark tile.

    briss.svg              the tile filling the canvas (Windows, Linux, the window icon, the README)
    briss-small.svg        the same, simplified for 16 to 32 pixels
    briss-macos.svg        on Apple's icon grid: an 824-point tile with a shadow
    briss-macos-small.svg  the macOS tile, simplified for 16 to 32 pixels

Run it from this folder: python3 svg.py
"""

ACCENT = "#4d8dff"  # colAccent in gui/theme.go


def art(small):
    """The mark on a 1024 canvas."""
    # the page, with a folded corner
    px0, py0, px1, py1, r, fold = 236, 136, 788, 888, 34, 132
    page = (f"M{px0+r} {py0} H{px1-fold} L{px1} {py0+fold} V{py1-r} "
            f"Q{px1} {py1} {px1-r} {py1} H{px0+r} Q{px0} {py1} {px0} {py1-r} V{py0+r} Q{px0} {py0} {px0+r} {py0} Z")
    foldp = f"M{px1-fold} {py0} V{py0+fold-30} Q{px1-fold} {py0+fold} {px1-fold+30} {py0+fold} H{px1} Z"
    # the crop: the part that is kept
    cx0, cy0, cx1, cy1 = 318, 300, 706, 790
    widths = [0.55, 1.0, 0.8, 1.0, 0.62] if small else [0.55, None, 1.0, 0.9, 1.0, 0.72, None, 1.0, 0.6]
    step, lh = (84, 46) if small else (44, 24)
    lines, y = [], cy0 + 62
    for i, w in enumerate(widths):
        if w is not None:
            x0 = cx0 + 52
            x1 = x0 + (cx1 - 52 - x0) * w
            fill = ACCENT if i == 0 else ("#98a2b3" if small else "#aab3c2")
            h = lh if small or i else 30
            lines.append(f'<rect x="{x0}" y="{y}" width="{x1-x0:.0f}" height="{h}" rx="{h/2:.0f}" fill="{fill}"/>')
        y += step
    arm, sw = (190, 76) if small else (176, 52)
    ghost = (".10", ".34", "10") if small else (".07", ".22", "6")
    o = 30  # the corners sit just outside the kept part
    tl = f"M{cx0-o} {cy0-o+arm} V{cy0-o} H{cx0-o+arm}"
    br = f"M{cx1+o} {cy1+o-arm} V{cy1+o} H{cx1+o-arm}"
    return f'''
    <path d="{page}" fill="#ffffff" fill-opacity="{ghost[0]}" stroke="#ffffff" stroke-opacity="{ghost[1]}" stroke-width="{ghost[2]}" stroke-linejoin="round"/>
    <path d="{foldp}" fill="#ffffff" fill-opacity=".14"/>
    <g filter="url(#pageShadow)"><rect x="{cx0}" y="{cy0}" width="{cx1-cx0}" height="{cy1-cy0}" rx="14" fill="url(#paper)"/></g>
    {"".join(lines)}
    <g filter="url(#glow)" stroke="url(#accent)" stroke-width="{sw}" stroke-linecap="round" stroke-linejoin="round" fill="none">
      <path d="{tl}"/><path d="{br}"/>
    </g>'''


DEFS = f'''
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#232a3a"/><stop offset=".55" stop-color="#12141b"/><stop offset="1" stop-color="#0b0b0d"/>
    </linearGradient>
    <radialGradient id="halo" cx=".5" cy=".42" r=".55">
      <stop offset="0" stop-color="{ACCENT}" stop-opacity=".30"/><stop offset="1" stop-color="{ACCENT}" stop-opacity="0"/>
    </radialGradient>
    <linearGradient id="paper" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#ffffff"/><stop offset="1" stop-color="#e6e9ef"/>
    </linearGradient>
    <linearGradient id="accent" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="#8ab8ff"/><stop offset=".5" stop-color="{ACCENT}"/><stop offset="1" stop-color="#2f6bea"/>
    </linearGradient>
    <linearGradient id="rim" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#ffffff" stop-opacity=".18"/><stop offset=".3" stop-color="#ffffff" stop-opacity="0"/>
    </linearGradient>
    <filter id="pageShadow" x="-20%" y="-20%" width="140%" height="140%">
      <feDropShadow dx="0" dy="22" stdDeviation="24" flood-color="#000" flood-opacity=".55"/>
    </filter>
    <filter id="glow" x="-30%" y="-30%" width="160%" height="160%">
      <feDropShadow dx="0" dy="0" stdDeviation="14" flood-color="{ACCENT}" flood-opacity=".65"/>
    </filter>
    <filter id="tileShadow" x="-20%" y="-20%" width="140%" height="140%">
      <feDropShadow dx="0" dy="12" stdDeviation="14" flood-color="#000" flood-opacity=".35"/>
    </filter>
    <clipPath id="tile"><rect width="1024" height="1024" rx="228"/></clipPath>
  </defs>'''


def svg(macos, small):
    body = f'''<g clip-path="url(#tile)">
    <rect width="1024" height="1024" fill="url(#bg)"/>
    <rect width="1024" height="1024" fill="url(#halo)"/>{art(small)}
  </g>
  <rect x="2" y="2" width="1020" height="1020" rx="226" fill="none" stroke="url(#rim)" stroke-width="4"/>'''
    if macos:
        body = f'<g transform="translate(100 100) scale(0.8046875)"><g filter="url(#tileShadow)">{body}</g></g>'
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">{DEFS}\n  {body}\n</svg>\n'


if __name__ == "__main__":
    for name, macos, small in [("briss", False, False), ("briss-small", False, True),
                               ("briss-macos", True, False), ("briss-macos-small", True, True)]:
        with open(name + ".svg", "w") as f:
            f.write(svg(macos, small))
