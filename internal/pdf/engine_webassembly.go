//go:build !pdfium_cgo

package pdf

import (
	"io"
	"os"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// newPool starts one wazero instance per worker. The runtime config must enable
// the same core features go-pdfium enables by default (its module uses
// exception handling); the compilation cache is added on top.
func newPool(workers int, cacheDir string) (pdfium.Pool, string, error) {
	rc := wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling)
	if cacheDir == "" {
		cacheDir = DefaultCacheDir()
	}
	if cacheDir != "-" {
		if err := os.MkdirAll(cacheDir, 0o755); err == nil {
			if cache, err := wazero.NewCompilationCacheWithDir(cacheDir); err == nil {
				rc = rc.WithCompilationCache(cache)
			}
		}
		// An unwritable cache directory only costs start-up time, so it is not an error.
	}
	pool, err := webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: workers, MaxTotal: workers, RuntimeConfig: rc,
		Stdout: stdio{os.Stdout}, Stderr: stdio{os.Stderr}})
	return pool, "webassembly", err
}

// stdio passes what PDFium prints on to w without handing wazero the
// *os.File itself. wazero inspects a file it is given, and in a program
// without usable standard handles (the Windows desktop app, built without a
// console) that fails, and with it every PDF: "could not instantiate
// webassembly module: GetFileType /dev/stdout: The handle is invalid".
// The output is only diagnostics, so write errors are dropped.
type stdio struct{ w io.Writer }

func (s stdio) Write(p []byte) (int, error) {
	if s.w != nil {
		s.w.Write(p)
	}
	return len(p), nil
}
