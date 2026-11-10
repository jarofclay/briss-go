# Decision records

One file per decision that shapes the rewrite. Each says what was decided, why,
what it costs, and what evidence would reopen it. Measurements are in
`spike/RESULTS.md` unless a record says otherwise.

| # | Decision | Status |
|---|---|---|
| [0001](0001-language-go.md) | Implement briss-go in Go | Accepted 2026-11-04 |
| [0002](0002-pdfium-webassembly.md) | Read and render with go-pdfium in WebAssembly mode | Accepted 2026-11-04 |
| [0003](0003-pdfcpu-in-place-cloning.md) | Write with pdfcpu by cloning pages in place | Accepted 2026-11-04 |
| [0004](0004-fyne-gui.md) | Build the GUI with Fyne | Accepted 2026-11-06 (in use; GPU frame check still open) |
| [0005](0005-tiered-detection.md) | Five detection tiers behind one interface | Accepted 2026-11-04 |
| [0006](0006-claude-choice-only.md) | Claude picks among local candidates, never edits coordinates | Accepted 2026-11-05 |
| [0007](0007-secrets.md) | No credentials in the repository | Accepted 2026-11-05 |
| [0008](0008-license-gpl.md) | Keep GPL-3.0 | Accepted 2026-11-04 |
| [0009](0009-legacy-parity.md) | Keep the legacy tier bit-compatible, fix quirks elsewhere | Accepted 2026-11-05 |
| [0010](0010-scan-tier-per-page.md) | The scan tier works page by page, from text lines | Accepted 2026-11-05 |
| [0011](0011-gui-drop-and-go.md) | The app is drop-and-go, dark and minimal, with review as an option | Accepted 2026-11-06 |
