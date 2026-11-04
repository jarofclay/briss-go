//go:build !pdfium_cgo

package main

import (
	"os"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

const backendName = "webassembly"

// newPool starts one wazero runtime per worker. When BRISS_WAZERO_CACHE names a
// directory, the compiled module is cached there so later starts skip compilation.
func newPool(workers int) (pdfium.Pool, error) {
	cfg := webassembly.Config{MinIdle: workers, MaxIdle: workers, MaxTotal: workers}
	if dir := os.Getenv("BRISS_WAZERO_CACHE"); dir != "" {
		cache, err := wazero.NewCompilationCacheWithDir(dir)
		if err != nil {
			return nil, err
		}
		// Same core features go-pdfium enables by default (its module needs
		// exception handling), plus the on-disk compilation cache.
		cfg.RuntimeConfig = wazero.NewRuntimeConfig().
			WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
			WithCompilationCache(cache)
	}
	return webassembly.Init(cfg)
}
