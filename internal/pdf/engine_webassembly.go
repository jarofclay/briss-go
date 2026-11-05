//go:build !pdfium_cgo

package pdf

import (
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
	pool, err := webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: workers, MaxTotal: workers, RuntimeConfig: rc})
	return pool, "webassembly", err
}
