// Package pdf reads and renders PDF documents through PDFium (go-pdfium).
//
// The default backend runs PDFium compiled to WebAssembly under wazero, so the
// binary needs no cgo and no system libraries. Build with -tags pdfium_cgo to
// link a native PDFium instead (see engine_cgo.go). Every other package talks to
// PDFium only through this package.
package pdf

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	pdfiumerrors "github.com/klippa-app/go-pdfium/errors"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

// Errors returned when opening a document.
var (
	// ErrPasswordRequired means the document is encrypted and no password was given.
	ErrPasswordRequired = errors.New("pdf: document is encrypted, a password is required")
	// ErrBadPassword means the given password does not open the document.
	ErrBadPassword = errors.New("pdf: wrong password")
)

// EngineOptions configures an Engine.
type EngineOptions struct {
	// Workers is the number of PDFium instances used for parallel rendering.
	// Zero selects DefaultWorkers().
	Workers int
	// CacheDir holds the compiled PDFium WebAssembly module so that later starts
	// skip the 4 to 6 second compile. Empty selects DefaultCacheDir(); "-"
	// disables the cache. Ignored by the cgo backend.
	CacheDir string
}

// Engine owns a pool of PDFium instances.
type Engine struct {
	pool    pdfium.Pool
	workers int
	backend string
}

// PageInfo describes one page as it is displayed (after /Rotate).
type PageInfo struct {
	Index  int     // 0-based page index
	Width  float64 // displayed width in points
	Height float64 // displayed height in points
}

// Document holds the metadata of an opened PDF. It does not keep a PDFium
// handle open: rendering opens the file on every worker instance.
type Document struct {
	Path     string
	Pages    []PageInfo
	password string
}

// NewEngine starts the PDFium pool.
func NewEngine(opts EngineOptions) (*Engine, error) {
	w := opts.Workers
	if w <= 0 {
		w = DefaultWorkers()
	}
	pool, backend, err := newPool(w, opts.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("pdf: starting PDFium: %w", err)
	}
	return &Engine{pool: pool, workers: w, backend: backend}, nil
}

// Close releases the PDFium pool.
func (e *Engine) Close() error { return e.pool.Close() }

// Workers returns the number of parallel render workers.
func (e *Engine) Workers() int { return e.workers }

// Backend names the PDFium backend in use ("webassembly" or "cgo").
func (e *Engine) Backend() string { return e.backend }

// DefaultCacheDir is the directory for the compiled WebAssembly module.
func DefaultCacheDir() string {
	if d := os.Getenv("BRISS_WAZERO_CACHE"); d != "" {
		return d
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return base + string(os.PathSeparator) + "briss-go" + string(os.PathSeparator) + "wazero"
}

// DefaultWorkers picks the render pool size: at most 4 and at most the number
// of CPUs, 2 on machines with 8 GB of memory or less, and never more than one
// instance per 150 MB of available memory (each WebAssembly instance holds its
// own linear memory; see PLAN.md section 4.4).
func DefaultWorkers() int {
	n := min(runtime.NumCPU(), 4)
	if total, avail, ok := memInfo(); ok {
		if total <= 8<<30 {
			n = min(n, 2)
		}
		if byMem := int(avail / (150 << 20)); byMem < n {
			n = byMem
		}
	}
	return max(n, 1)
}

// memInfo reads total and available memory on Linux; ok is false elsewhere.
func memInfo() (total, avail uint64, ok bool) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = v << 10
		case "MemAvailable:":
			avail = v << 10
		}
	}
	return total, avail, total > 0 && avail > 0
}

func (e *Engine) instance(ctx context.Context) (pdfium.Pdfium, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
	}
	return e.pool.GetInstanceWithContext(ctx)
}

func openDoc(inst pdfium.Pdfium, path, password string) (references.FPDF_DOCUMENT, error) {
	req := &requests.OpenDocument{FilePath: &path}
	if password != "" {
		req.Password = &password
	}
	res, err := inst.OpenDocument(req)
	if err != nil {
		if errors.Is(err, pdfiumerrors.ErrPassword) {
			if password == "" {
				return "", ErrPasswordRequired
			}
			return "", ErrBadPassword
		}
		return "", fmt.Errorf("pdf: opening %s: %w", path, err)
	}
	return res.Document, nil
}

// Open reads the page sizes of a document. password may be empty.
func (e *Engine) Open(ctx context.Context, path, password string) (*Document, error) {
	inst, err := e.instance(ctx)
	if err != nil {
		return nil, err
	}
	defer inst.Close()
	ref, err := openDoc(inst, path, password)
	if err != nil {
		return nil, err
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: ref})
	pc, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: ref})
	if err != nil {
		return nil, fmt.Errorf("pdf: page count: %w", err)
	}
	doc := &Document{Path: path, password: password, Pages: make([]PageInfo, pc.PageCount)}
	for i := 0; i < pc.PageCount; i++ {
		// FPDF_GetPageSizeByIndex reports the displayed size (rotation applied)
		// without loading the page; the F variant is experimental on cgo.
		sz, err := inst.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{Document: ref, Index: i})
		if err != nil {
			return nil, fmt.Errorf("pdf: size of page %d: %w", i+1, err)
		}
		doc.Pages[i] = PageInfo{Index: i, Width: sz.Width, Height: sz.Height}
	}
	return doc, nil
}

// RenderJob asks for one page rendered to gray at Width x Height pixels.
type RenderJob struct {
	Page   int // 0-based page index
	Width  int
	Height int
	Tag    int // caller-defined, passed back unchanged (for example a cluster index)
}

// Render renders jobs in parallel on the engine's workers and calls fn for each
// result. fn is called concurrently from several goroutines and must not keep
// img after it returns: in WebAssembly mode its pixel buffer is released. The
// image can be up to a pixel smaller than requested in one dimension because
// PDFium keeps the page aspect ratio. Annotations are rendered, like the Java
// Briss preview.
func (e *Engine) Render(ctx context.Context, doc *Document, jobs []RenderJob, fn func(RenderJob, *image.Gray) error) error {
	if len(jobs) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		errOnce  sync.Once
		firstErr error
	)
	fail := func(err error) {
		errOnce.Do(func() { firstErr = err; cancel() })
	}
	jobCh := make(chan RenderJob)
	var wg sync.WaitGroup
	for w := 0; w < min(e.workers, len(jobs)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.renderWorker(ctx, doc, jobCh, fn); err != nil {
				fail(err)
				for range jobCh { // keep the feeder from blocking
				}
			}
		}()
	}
feed:
	for _, j := range jobs {
		select {
		case jobCh <- j:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobCh)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func (e *Engine) renderWorker(ctx context.Context, doc *Document, jobs <-chan RenderJob, fn func(RenderJob, *image.Gray) error) error {
	inst, err := e.instance(ctx)
	if err != nil {
		return err
	}
	defer inst.Close()
	ref, err := openDoc(inst, doc.Path, doc.password)
	if err != nil {
		return err
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: ref})
	for j := range jobs {
		if ctx.Err() != nil {
			continue
		}
		res, err := inst.RenderPageInPixels(&requests.RenderPageInPixels{
			Page:        requests.Page{ByIndex: &requests.PageByIndex{Document: ref, Index: j.Page}},
			Width:       j.Width,
			Height:      j.Height,
			RenderFlags: enums.FPDF_RENDER_FLAG_ANNOT,
			ImageFormat: requests.RenderImageFormatGrayscale,
		})
		if err != nil {
			return fmt.Errorf("pdf: rendering page %d: %w", j.Page+1, err)
		}
		g, ok := res.Result.RenderedImage.(*image.Gray)
		if !ok {
			if res.CleanupFunc != nil {
				res.CleanupFunc()
			}
			return fmt.Errorf("pdf: rendering page %d: unexpected image type %T", j.Page+1, res.Result.RenderedImage)
		}
		err = fn(j, g)
		if res.CleanupFunc != nil {
			res.CleanupFunc()
		}
		if err != nil {
			return err
		}
	}
	return nil
}
