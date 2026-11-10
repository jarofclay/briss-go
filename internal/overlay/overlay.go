// Package overlay blends the sampled pages of a cluster into one gray image,
// exactly like the Java ClusterImageData: each pixel becomes 255 minus the
// standard deviation of that pixel across pages, so content that moves between
// pages shows dark and content identical on every page (running headers, rules)
// stays light. When most inked pixels are identical on all pages the result
// would be blank, so the per-pixel minimum (darkest) is used instead.
//
// The accumulation is streaming: pages can be added from several goroutines as
// they are rendered, and none of them is kept.
package overlay

import (
	"image"
	"math"
	"sync"
)

// MaxHeight caps the overlay height in pixels (Java MAX_PAGE_HEIGHT).
const MaxHeight = 900

// IdenticalPixelsThreshold is Java's IDENTICAL_PIXELS_THRESHOLD.
const IdenticalPixelsThreshold = 0.8

// TargetSize returns the overlay size for a page of the given displayed size in
// points. It reproduces Java Briss: PDFBox renders at 300 DPI (pixel sizes
// floored from float32 products), the height is capped at MaxHeight, and the
// width is scaled with the same float32 factor and truncated.
func TargetSize(widthPt, heightPt float64) (w, h int) {
	scale := float32(300) / 72
	wpx := max(int(math.Floor(float64(float32(widthPt)*scale))), 1)
	hpx := max(int(math.Floor(float64(float32(heightPt)*scale))), 1)
	h = min(hpx, MaxHeight)
	sf := float32(h) / float32(hpx)
	w = max(int(float32(wpx)*sf), 1)
	return w, h
}

// Accumulator collects pages of one cluster.
type Accumulator struct {
	W, H  int
	mu    sync.Mutex
	n     int
	sum   []uint32
	sumsq []uint32 // at most 65025 per page, so up to 66,000 pages fit
	min   []uint8
}

// NewAccumulator prepares an overlay of w x h pixels.
func NewAccumulator(w, h int) *Accumulator {
	a := &Accumulator{W: w, H: h, sum: make([]uint32, w*h), sumsq: make([]uint32, w*h), min: make([]uint8, w*h)}
	for i := range a.min {
		a.min[i] = 255
	}
	return a
}

// Count returns the number of pages added.
func (a *Accumulator) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.n
}

// Add blends one rendered page. Pages of a different size are resampled with
// nearest neighbour, as Java's AffineTransform scaling did. Safe for
// concurrent use; g is not retained.
func (a *Accumulator) Add(g *image.Gray) {
	b := g.Bounds()
	gw, gh := b.Dx(), b.Dy()
	if gw == 0 || gh == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for y := 0; y < a.H; y++ {
		sy := y
		if gh != a.H {
			sy = min(y*gh/a.H, gh-1)
		}
		row := g.Pix[sy*g.Stride : sy*g.Stride+gw]
		base := y * a.W
		for x := 0; x < a.W; x++ {
			sx := x
			if gw != a.W {
				sx = min(x*gw/a.W, gw-1)
			}
			v := row[sx]
			i := base + x
			a.sum[i] += uint32(v)
			a.sumsq[i] += uint32(v) * uint32(v)
			if v < a.min[i] {
				a.min[i] = v
			}
		}
	}
	a.n++
}

// Image returns the overlay. It reproduces calculateOverlayOfImages, including
// its integer mean and truncated square root; with a single page it returns
// that page (Java's formula divides by zero there and falls back to the minimum).
func (a *Accumulator) Image() *image.Gray {
	a.mu.Lock()
	defer a.mu.Unlock()
	img := image.NewGray(image.Rect(0, 0, a.W, a.H))
	if a.n < 2 {
		copy(img.Pix, a.min)
		return img
	}
	n := int64(a.n)
	identical, white := 0, 0
	for i := range a.sum {
		s := int64(a.sum[i])
		mean := s / n
		// sum over pages of (p - mean)^2 with Java's integer mean
		dev := int64(a.sumsq[i]) - 2*mean*s + n*mean*mean
		sd := 255 - int(math.Sqrt(float64(dev)/float64(n-1)))
		img.Pix[i] = uint8(sd)
		if mean < 255 {
			if sd == 255 {
				identical++
			}
		} else {
			white++
		}
	}
	if float64(identical) > float64(a.W*a.H)*IdenticalPixelsThreshold-float64(white) {
		copy(img.Pix, a.min)
	}
	return img
}

// Profiles returns the mean gray value of every column and every row, used by
// the golden tests to compare overlays with the Java build.
func Profiles(g *image.Gray) (cols, rows []float64) {
	b := g.Bounds()
	w, h := b.Dx(), b.Dy()
	cols, rows = make([]float64, w), make([]float64, h)
	for y := 0; y < h; y++ {
		row := g.Pix[y*g.Stride : y*g.Stride+w]
		var s float64
		for x, v := range row {
			cols[x] += float64(v)
			s += float64(v)
		}
		rows[y] = s / float64(w)
	}
	for x := range cols {
		cols[x] /= float64(h)
	}
	return cols, rows
}
