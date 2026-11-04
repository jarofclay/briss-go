# Week-1 spike programs

Throwaway code that answers the exit criteria in `PLAN.md` section 3.4. Results are
in `RESULTS.md`. Each directory is its own Go module so nothing here leaks into the
future application module.

| Directory | Spike | Run |
|---|---|---|
| `tools/gen_corpus.py` | synthetic corpus with exact labels (290-page born-digital book, 118 single-page scans, 59 two-up scans) | `python tools/gen_corpus.py testdata` (needs reportlab, pypdfium2, pillow, numpy) |
| `render/` | 1 and 2: go-pdfium rendering benchmark, WebAssembly and cgo backends | `go build -o render . && ./render -pdf ../testdata/book300.pdf -workers 4 -out ../testdata/overlays -dump-samples 2` ; cgo: `PKG_CONFIG_PATH=<dir with pdfium.pc> go build -tags pdfium_cgo` ; set `BRISS_WAZERO_CACHE=<dir>` to cache the compiled module |
| `cropwrite/` | 3: pdfcpu in-place page cloning and box writing | `go build -o cropwrite . && ./cropwrite -in ../testdata/book300.pdf -out /tmp/out.pdf -labels ../testdata/labels.json` then `python ../tools/verify_crop.py ...` |
| `gui/` | 4: Fyne crop-editor widget | `go run . ../testdata/overlays/book300_c00_odd_612x792_p1.png` on a desktop; `go test -run TestEditorHeadless -v .` renders a screenshot headlessly |
| `tools/onnx_eval.py` | 5: layout-model latency and crop accuracy (CDLA PicoDet bundled with the rapid_layout wheel) | `python tools/onnx_eval.py testdata 40` (needs onnxruntime, rapid-layout) |
| `claude/` | 6: propose-and-choose harness for Claude | `go build -o claude-spike . && ./claude-spike -mode dry` ; `-mode live` needs `ANTHROPIC_API_KEY` |

Native PDFium for the cgo build came from the `pypdfium2` wheel (`pypdfium2_raw/libpdfium.so`)
with the public headers from the `chromium/pdfium` mirror; ONNX Runtime came from the
`onnxruntime` wheel. Both are workarounds for an environment where GitHub release
downloads are blocked, not a recommended distribution path.
