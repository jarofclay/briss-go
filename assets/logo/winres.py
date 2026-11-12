#!/usr/bin/env python3
"""Write the Windows icon of Briss as .syso files, so that `go build` puts it
into briss.exe and Briss.exe on its own (the Go linker picks up any
rsrc_windows_<cpu>.syso file in the package folder, also when cross-compiling).

Each file is a COFF object with one .rsrc section holding the icon images
(RT_ICON, PNG-compressed, which Windows reads since Vista) and the icon group
that points at them (RT_GROUP_ICON). Only the Python standard library is used.

Run it from this folder after render.js: python3 winres.py
"""
import struct

SIZES = [16, 24, 32, 48, 64, 128, 256]
TARGETS = {  # where the files go, and for which CPUs
    "../../cmd/briss": ["amd64", "arm64"],
    "../../gui": ["amd64", "arm64"],
}
MACHINE = {"amd64": (0x8664, 0x0003), "arm64": (0xAA64, 0x0002)}  # machine, ADDR32NB relocation
RT_ICON, RT_GROUP_ICON, LANG_EN_US = 3, 14, 0x0409


def resource_section(tree):
    """Lay out a resource tree {type: {id: data}} (one language each) and return
    the section bytes and the offsets of the data-entry fields to relocate."""
    types = sorted(tree)
    dir_size = lambda n: 16 + 8 * n
    # directories first: the root, one per type, one per (type, id) for the language
    off = dir_size(len(types))
    type_off, name_off = {}, {}
    for t in types:
        type_off[t] = off
        off += dir_size(len(tree[t]))
    for t in types:
        for i in sorted(tree[t]):
            name_off[t, i] = off
            off += dir_size(1)
    entry_off = {}
    for t in types:
        for i in sorted(tree[t]):
            entry_off[t, i] = off
            off += 16
    data_off = {}
    for t in types:
        for i in sorted(tree[t]):
            off = (off + 7) & ~7
            data_off[t, i] = off
            off += len(tree[t][i])
    sect = bytearray(off)

    def directory(at, ids_and_targets):
        struct.pack_into("<IIHHHH", sect, at, 0, 0, 0, 0, 0, len(ids_and_targets))
        for k, (ident, target) in enumerate(ids_and_targets):
            struct.pack_into("<II", sect, at + 16 + 8 * k, ident, target)

    directory(0, [(t, 0x80000000 | type_off[t]) for t in types])
    relocs = []
    for t in types:
        directory(type_off[t], [(i, 0x80000000 | name_off[t, i]) for i in sorted(tree[t])])
        for i in sorted(tree[t]):
            directory(name_off[t, i], [(LANG_EN_US, entry_off[t, i])])
            data = tree[t][i]
            struct.pack_into("<IIII", sect, entry_off[t, i], data_off[t, i], len(data), 0, 0)
            relocs.append(entry_off[t, i])
            sect[data_off[t, i]:data_off[t, i] + len(data)] = data
    return bytes(sect), relocs


def coff(section, relocs, machine, reloc_type):
    """Wrap a .rsrc section in a COFF object file with one section symbol."""
    hdr, shdr = 20, 40
    raw_at = hdr + shdr
    reloc_at = raw_at + len(section)
    sym_at = reloc_at + 10 * len(relocs)
    out = bytearray()
    out += struct.pack("<HHIIIHH", machine, 1, 0, sym_at, 1, 0, 0)
    out += struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(section), raw_at, reloc_at, 0, len(relocs), 0,
                       0x40000040)  # initialized data, readable
    out += section
    for r in relocs:
        out += struct.pack("<IIH", r, 0, reloc_type)  # relative to symbol 0, the section
    out += struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 0)  # static symbol of section 1
    out += struct.pack("<I", 4)  # empty string table
    return bytes(out)


def icon_tree():
    images, group = {}, bytearray(struct.pack("<HHH", 0, 1, len(SIZES)))
    for n, size in enumerate(SIZES, start=1):
        with open(f"png/briss-{size}.png", "rb") as f:
            png = f.read()
        images[n] = png
        side = 0 if size >= 256 else size
        group += struct.pack("<BBBBHHIH", side, side, 0, 0, 1, 32, len(png), n)
    return {RT_ICON: images, RT_GROUP_ICON: {1: bytes(group)}}


if __name__ == "__main__":
    section, relocs = resource_section(icon_tree())
    for folder, cpus in TARGETS.items():
        for cpu in cpus:
            machine, reloc_type = MACHINE[cpu]
            with open(f"{folder}/rsrc_windows_{cpu}.syso", "wb") as f:
                f.write(coff(section, relocs, machine, reloc_type))
