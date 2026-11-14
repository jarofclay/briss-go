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
	base := wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling)
	start := func(rc wazero.RuntimeConfig) (pdfium.Pool, error) {
		return webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: workers, MaxTotal: workers, RuntimeConfig: rc,
			Stdout: stdio{os.Stdout}, Stderr: stdio{os.Stderr}})
	}
	if cacheDir == "" {
		cacheDir = DefaultCacheDir()
	}
	if cacheDir != "-" {
		if err := os.MkdirAll(cacheDir, 0o755); err == nil {
			if cache, err := wazero.NewCompilationCacheWithDir(cacheDir); err == nil {
				if pool, err := start(base.WithCompilationCache(cache)); err == nil {
					return pool, "webassembly", nil
				}
				// The cache only saves start-up time, so a cache that cannot be
				// read or written must not stop the engine: on Windows another
				// process writing the same entry at that moment makes the rename
				// fail with "Access is denied". Compile without it instead.
			}
		}
		// An unwritable cache directory only costs start-up time, so it is not an error.
	}
	pool, err := start(base)
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
