//go:build pdfium_cgo

package pdf

import (
	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/single_threaded"
)

// newPool links the native PDFium found through pkg-config. go-pdfium's
// single-threaded backend serialises calls, so extra workers only wait; it is
// still 2 to 3x faster per page than WebAssembly (spike/RESULTS.md).
func newPool(workers int, _ string) (pdfium.Pool, string, error) {
	return single_threaded.Init(single_threaded.Config{}), "cgo", nil
}
