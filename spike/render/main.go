// Spike 1/2: render sampled cluster pages in parallel with go-pdfium and build
// briss-style overlays. Reports pool startup, per-page render latency, throughput
// and peak RSS. Backend selected by build tag: default WebAssembly, -tags pdfium_cgo native.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium/requests"
)

type cluster struct {
	even   bool
	w, h   float64
	pages  []int // 0-based
	sample []int
	// accumulator
	mu     sync.Mutex
	tw, th int
	count  int
	sum    []float64
	sumsq  []float64
	min    []uint8
}

type job struct {
	c    *cluster
	ci   int // cluster index
	ord  int // position within the cluster sample
	page int
}

func main() {
	pdfPath := flag.String("pdf", "", "PDF to process")
	workers := flag.Int("workers", runtime.NumCPU(), "parallel pdfium instances")
	maxH := flag.Int("height", 900, "overlay height in px")
	maxSample := flag.Int("sample", 15, "max pages rendered per cluster")
	outDir := flag.String("out", "", "write overlay PNGs here (optional)")
	dumpSamples := flag.Int("dump-samples", 0, "also write the first N rendered sample pages per cluster as PNG")
	password := flag.String("password", "", "PDF password")
	flag.Parse()
	if *pdfPath == "" {
		fmt.Fprintln(os.Stderr, "usage: render -pdf file.pdf [-workers N] [-height 900] [-out dir]")
		os.Exit(1)
	}
	var pw *string
	if *password != "" {
		pw = password
	}

	// --- pool startup
	t0 := time.Now()
	pool, err := newPool(*workers)
	must(err)
	defer pool.Close()
	inst, err := pool.GetInstance(60 * time.Second)
	must(err)
	tStartup := time.Since(t0)

	// --- metadata only: page sizes
	t1 := time.Now()
	doc, err := inst.OpenDocument(&requests.OpenDocument{FilePath: pdfPath, Password: pw})
	must(err)
	pc, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	must(err)
	var clusters []*cluster
	for i := 0; i < pc.PageCount; i++ {
		// FPDF_GetPageSizeByIndex is the non-experimental variant (the F variant
		// needs the pdfium_experimental build tag on the cgo backend).
		sz, err := inst.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{Document: doc.Document, Index: i})
		must(err)
		w, h := sz.Width, sz.Height
		even := (i+1)%2 == 0
		var found *cluster
		for _, c := range clusters {
			if c.even == even && math.Abs(c.w-w) <= 2 && math.Abs(c.h-h) <= 2 {
				found = c
				break
			}
		}
		if found == nil {
			found = &cluster{even: even, w: w, h: h}
			clusters = append(clusters, found)
		}
		found.pages = append(found.pages, i)
	}
	_, _ = inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	must(inst.Close())
	tMeta := time.Since(t1)

	base := strings.TrimSuffix(filepath.Base(*pdfPath), filepath.Ext(*pdfPath))
	if *outDir != "" {
		must(os.MkdirAll(*outDir, 0o755))
	}
	var jobs []job
	for ci, c := range clusters {
		if len(c.pages) <= *maxSample {
			c.sample = c.pages
		} else {
			step := float64(len(c.pages)) / float64(*maxSample)
			for k := 0; k < *maxSample; k++ {
				c.sample = append(c.sample, c.pages[int(math.Floor(float64(k)*step))])
			}
		}
		scale := float64(*maxH) / c.h
		c.th = *maxH
		c.tw = int(math.Round(c.w * scale))
		n := c.tw * c.th
		c.sum = make([]float64, n)
		c.sumsq = make([]float64, n)
		c.min = make([]uint8, n)
		for i := range c.min {
			c.min[i] = 255
		}
		for ord, p := range c.sample {
			jobs = append(jobs, job{c: c, ci: ci, ord: ord, page: p})
		}
	}

	// --- parallel render
	t2 := time.Now()
	jobCh := make(chan job)
	var wg sync.WaitGroup
	var statMu sync.Mutex
	var renderTimes []time.Duration
	var openTimes []time.Duration
	for wkr := 0; wkr < *workers; wkr++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst, err := pool.GetInstance(60 * time.Second)
			must(err)
			defer inst.Close()
			to := time.Now()
			doc, err := inst.OpenDocument(&requests.OpenDocument{FilePath: pdfPath, Password: pw})
			must(err)
			openDur := time.Since(to)
			defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
			var mine []time.Duration
			for j := range jobCh {
				tr := time.Now()
				res, err := inst.RenderPageInPixels(&requests.RenderPageInPixels{
					Page:        requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: j.page}},
					Width:       j.c.tw,
					Height:      j.c.th,
					ImageFormat: requests.RenderImageFormatGrayscale,
				})
				must(err)
				g, ok := res.Result.RenderedImage.(*image.Gray)
				if !ok {
					panic("expected *image.Gray")
				}
				accumulate(j.c, g)
				if *outDir != "" && *dumpSamples > 0 && j.ord < *dumpSamples {
					name := fmt.Sprintf("%s_c%02d_page%03d.png", base, j.ci, j.page+1)
					f, err := os.Create(filepath.Join(*outDir, name))
					must(err)
					must(png.Encode(f, g))
					f.Close()
				}
				if res.CleanupFunc != nil {
					res.CleanupFunc()
				}
				mine = append(mine, time.Since(tr))
			}
			statMu.Lock()
			renderTimes = append(renderTimes, mine...)
			openTimes = append(openTimes, openDur)
			statMu.Unlock()
		}()
	}
	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)
	wg.Wait()
	tRender := time.Since(t2)

	// --- overlays
	t3 := time.Now()
	for ci, c := range clusters {
		img := overlay(c)
		if *outDir != "" {
			name := fmt.Sprintf("%s_c%02d_%s_%dx%d_p%d.png", base, ci, map[bool]string{true: "even", false: "odd"}[c.even], int(c.w), int(c.h), c.pages[0]+1)
			f, err := os.Create(filepath.Join(*outDir, name))
			must(err)
			must(png.Encode(f, img))
			f.Close()
		}
	}
	tOverlay := time.Since(t3)

	// --- report
	sort.Slice(renderTimes, func(i, j int) bool { return renderTimes[i] < renderTimes[j] })
	p := func(q float64) time.Duration { return renderTimes[int(float64(len(renderTimes)-1)*q)] }
	var mean time.Duration
	for _, d := range renderTimes {
		mean += d
	}
	mean /= time.Duration(len(renderTimes))
	var maxOpen time.Duration
	for _, d := range openTimes {
		if d > maxOpen {
			maxOpen = d
		}
	}
	fmt.Printf("backend=%s file=%s pages=%d clusters=%d rendered=%d workers=%d height=%d\n",
		backendName, filepath.Base(*pdfPath), pc.PageCount, len(clusters), len(renderTimes), *workers, *maxH)
	for ci, c := range clusters {
		fmt.Printf("  cluster %d: %s %dx%dpt pages=%d sampled=%d overlay=%dx%d\n", ci,
			map[bool]string{true: "even", false: "odd"}[c.even], int(c.w), int(c.h), len(c.pages), len(c.sample), c.tw, c.th)
	}
	fmt.Printf("startup(pool+instance)=%v meta(open+sizes)=%v open_per_worker_max=%v\n", tStartup.Round(time.Millisecond), tMeta.Round(time.Millisecond), maxOpen.Round(time.Millisecond))
	fmt.Printf("render: p50=%v p95=%v max=%v mean=%v wall=%v throughput=%.1f pages/s\n",
		p(0.5).Round(time.Millisecond), p(0.95).Round(time.Millisecond), p(1).Round(time.Millisecond), mean.Round(time.Millisecond),
		tRender.Round(time.Millisecond), float64(len(renderTimes))/tRender.Seconds())
	fmt.Printf("overlay(all clusters)=%v total=%v peakRSS=%s\n", tOverlay.Round(time.Millisecond), time.Since(t0).Round(time.Millisecond), peakRSS())
}

func accumulate(c *cluster, g *image.Gray) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := g.Bounds()
	w, h := b.Dx(), b.Dy()
	for y := 0; y < c.th; y++ {
		sy := y
		if h != c.th {
			sy = y * h / c.th
		}
		row := g.Pix[sy*g.Stride : sy*g.Stride+w]
		for x := 0; x < c.tw; x++ {
			sx := x
			if w != c.tw {
				sx = x * w / c.tw
			}
			v := row[sx]
			i := y*c.tw + x
			fv := float64(v)
			c.sum[i] += fv
			c.sumsq[i] += fv * fv
			if v < c.min[i] {
				c.min[i] = v
			}
		}
	}
	c.count++
}

// overlay reproduces ClusterImageData.calculateOverlayOfImages: 255 - sd per pixel,
// falling back to the per-pixel minimum when most non-white pixels are identical.
func overlay(c *cluster) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, c.tw, c.th))
	n := float64(c.count)
	if c.count < 2 {
		copy(img.Pix, c.min)
		return img
	}
	identical, white := 0, 0
	for i := range c.sum {
		mean := c.sum[i] / n
		varc := (c.sumsq[i] - n*mean*mean) / (n - 1)
		if varc < 0 {
			varc = 0
		}
		sd := 255 - int(math.Sqrt(varc))
		if sd < 0 {
			sd = 0
		}
		img.Pix[i] = uint8(sd)
		if int(mean) < 255 {
			if sd == 255 {
				identical++
			}
		} else {
			white++
		}
	}
	if float64(identical) > float64(len(c.sum))*0.8-float64(white) {
		copy(img.Pix, c.min)
	}
	return img
}

func peakRSS() string {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return "n/a"
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "VmHWM:") {
			return strings.TrimSpace(strings.TrimPrefix(sc.Text(), "VmHWM:"))
		}
	}
	return "n/a"
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
