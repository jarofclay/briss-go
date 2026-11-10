//go:build pdfium_cgo

package main

import (
	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/single_threaded"
)

const backendName = "cgo-single-threaded"

// single_threaded serialises every call through one PDFium library instance,
// so workers > 1 only measures lock contention.
func newPool(workers int) (pdfium.Pool, error) {
	return single_threaded.Init(single_threaded.Config{}), nil
}
