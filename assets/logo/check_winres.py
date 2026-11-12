#!/usr/bin/env python3
"""Check that Windows programs carry the Briss icon: the icon group and every
image in it must match png/briss-<size>.png byte for byte.

    python3 assets/logo/check_winres.py dist/Briss.exe [more.exe ...]

Only the Python standard library is used, so it runs on any CI machine.
"""
import os
import struct
import sys

PNG_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "png")
RT_ICON, RT_GROUP_ICON = 3, 14


def resources(path):
    d = open(path, "rb").read()
    pe = struct.unpack_from("<I", d, 0x3C)[0]
    if d[pe:pe + 4] != b"PE\0\0":
        raise ValueError("not a PE file")
    nsect, optsize = struct.unpack_from("<H", d, pe + 6)[0], struct.unpack_from("<H", d, pe + 20)[0]
    opt = pe + 24
    dd = opt + (96 if struct.unpack_from("<H", d, opt)[0] == 0x10B else 112)
    rsrc_rva = struct.unpack_from("<I", d, dd + 8 * 2)[0]
    if rsrc_rva == 0:
        raise ValueError("no resources")
    sects = []
    for k in range(nsect):
        _, vsize, va, rawsize, rawptr = struct.unpack_from("<8sIIII", d, opt + optsize + 40 * k)
        sects.append((va, max(vsize, rawsize), rawptr))

    def off(rva):
        for va, size, rawptr in sects:
            if va <= rva < va + size:
                return rawptr + rva - va
        raise ValueError(f"address {rva:#x} outside the sections")

    base = off(rsrc_rva)

    def entries(o):
        n = sum(struct.unpack_from("<HH", d, base + o + 12))
        return [struct.unpack_from("<II", d, base + o + 16 + 8 * k) for k in range(n)]

    res = {}
    for t, tt in entries(0):
        for i, it in entries(tt & 0x7FFFFFFF):
            for _, lt in entries(it & 0x7FFFFFFF):
                rva, size = struct.unpack_from("<II", d, base + lt)
                res[t, i] = d[off(rva):off(rva) + size]
    return res


def check(path):
    res = resources(path)
    groups = [v for (t, _), v in res.items() if t == RT_GROUP_ICON]
    if not groups:
        raise ValueError("no icon group")
    _, _, count = struct.unpack_from("<HHH", groups[0], 0)
    sizes = []
    for k in range(count):
        w, _, _, _, _, _, nbytes, nid = struct.unpack_from("<BBBBHHIH", groups[0], 6 + 14 * k)
        side = w or 256
        with open(os.path.join(PNG_DIR, f"briss-{side}.png"), "rb") as f:
            want = f.read()
        if res.get((RT_ICON, nid)) != want or nbytes != len(want):
            raise ValueError(f"the {side}-pixel icon differs from png/briss-{side}.png")
        sizes.append(side)
    return sizes


if __name__ == "__main__":
    failed = False
    for p in sys.argv[1:]:
        try:
            print(f"{p}: Briss icon at {', '.join(map(str, check(p)))} pixels")
        except (OSError, ValueError, struct.error) as e:
            print(f"{p}: {e}", file=sys.stderr)
            failed = True
    sys.exit(1 if failed or len(sys.argv) < 2 else 0)
