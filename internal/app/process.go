package app

import (
	"context"
	"fmt"
	"image"
	"math"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/jarofclay/briss-go/internal/crop"
	"github.com/jarofclay/briss-go/internal/cropdef"
	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/jarofclay/briss-go/internal/ocr"
	"github.com/jarofclay/briss-go/internal/pdf"
	"github.com/jarofclay/briss-go/internal/raster"
)

// Extras are the steps beyond cropping that the GUI turns on by default.
type Extras struct {
	Straighten bool   // turn leaning scans level (angles from the scan detector)
	Normalize  bool   // give every output page of a kind the same size
	OCR        bool   // lay an invisible text layer over scanned pages (needs Tesseract)
	OCRLang    string // Tesseract languages, e.g. "eng" or "eng+deu"
}

// Progress reports a stage ("detect", "ocr", "write") and how far it got.
type Progress func(stage string, done, total int)

// Straightening limits: smaller angles are noise, larger ones are not skew.
const (
	minTurn = 0.1 // degrees
	maxTurn = 5.0
)

// Outputs turns a crop definition into the pages to write: rectangles in
// order, straightened and normalised as ex asks. pages are the source pages
// as displayed (sizes in points).
func Outputs(def *cropdef.Definition, pages []pdf.PageInfo, ex Extras) (map[int][]crop.Output, error) {
	po, err := def.PageOutputs()
	if err != nil {
		return nil, err
	}
	outs := make(map[int][]crop.Output, len(po))
	for p, rs := range po {
		if p < 1 || p > len(pages) {
			return nil, fmt.Errorf("%w: page %d out of range", ErrInput, p)
		}
		pi := pages[p-1]
		for _, r := range rs {
			o := crop.Output{Rect: r.Ratios(), Turn: r.Turn}
			if a := r.Angle; ex.Straighten && math.Abs(a) >= minTurn && math.Abs(a) <= maxTurn {
				if sr, ok := straightRect(o.Rect, a, pi.Width, pi.Height); ok {
					px, py := o.PivotPoint() // the content's centre, kept when normalising moves the box
					o.Rect, o.Angle, o.Pivot = sr, a, &[2]float64{px, py}
				}
			}
			outs[p] = append(outs[p], o)
		}
	}
	if ex.Normalize {
		normalize(outs, pages)
	}
	return outs, nil
}

// straightRect returns the rectangle of the content inside r once it is
// turned level: a block of w x h points tilted by deg fills a box of
// W = w cos + h sin by H = w sin + h cos, so the straight block, about the
// same centre, is the solution of that pair.
func straightRect(r geom.Ratios, deg, pw, ph float64) (geom.Ratios, bool) {
	s, c := math.Sincos(math.Abs(deg) * math.Pi / 180)
	W, H := (1-r.L-r.R)*pw, (1-r.T-r.B)*ph
	den := c*c - s*s
	w, h := (W*c-H*s)/den, (H*c-W*s)/den
	if den <= 0 || w < 0.5*W || h < 0.5*H {
		return r, false
	}
	cx, cy := (r.L+(1-r.R))/2*pw, (r.T+(1-r.B))/2*ph
	out := geom.Ratios{L: (cx - w/2) / pw, R: 1 - (cx+w/2)/pw, T: (cy - h/2) / ph, B: 1 - (cy+h/2)/ph}
	return out, out.Valid()
}

// normalize gives every output page the size of the largest one, so an
// e-reader shows every page at the same scale and a printout has one paper
// size. Landscape pages (a fold-out map among book pages) are sized among
// themselves, so they do not make every page as wide as they are; what counts
// is the shape of the place the output comes from, so each half of a split
// two-page spread is a portrait page. Each rectangle first grows over its own page: horizontally about its
// centre, never past half way to a neighbouring rectangle of the same sheet
// (the facing page of a split spread, the other column); vertically it keeps
// its top when its top is where most pages of its kind have theirs (a short
// last page of a chapter), its bottom likewise (a chapter opening), and grows
// about its centre otherwise. Where the page has no more room, the output page
// is made larger than what it shows (Output.Box) and the rest stays blank, so
// the neighbour's content never shows. A kind is the source page size, the
// left or right half of a landscape sheet, and the top or bottom of a page
// split into rows; it only decides the alignment, not the size.
func normalize(outs map[int][]crop.Output, pages []pdf.PageInfo) {
	// Work on every page as it will be read: turned upright when its output
	// carries a turn, so "top", "left page" and the sizes mean what a reader
	// sees.
	type view struct {
		r      geom.Ratios // the rectangle on the upright page
		pw, ph float64     // the upright page's size
	}
	views := map[int][]view{}
	for p, os := range outs {
		pi := pages[p-1]
		for _, o := range os {
			v := view{r: o.Rect.Turn(o.Turn), pw: pi.Width, ph: pi.Height}
			if o.Turn%180 == 90 {
				v.pw, v.ph = pi.Height, pi.Width
			}
			views[p] = append(views[p], v)
		}
	}
	type ref struct{ page, idx int }
	// over reports whether a and b share some of their extent along one axis
	over := func(a0, a1, b0, b1 float64) bool { return a0 < b1 && b0 < a1 }
	// room is the place an output may fill: its page, up to half way to the
	// neighbours beside, above and below it
	type span struct{ lo, hi, loY, hiY float64 }
	rooms := map[ref]span{}
	var size [2][2]float64 // the size every output page gets (width, height), portrait and landscape
	landscape := map[ref]int{}
	for p, vs := range views {
		for i, v := range vs {
			x0, x1 := v.r.L*v.pw, (1-v.r.R)*v.pw
			y0, y1 := v.r.T*v.ph, (1-v.r.B)*v.ph
			rm := span{0, v.pw, 0, v.ph}
			for j, n := range vs {
				if j == i {
					continue
				}
				nx0, nx1 := n.r.L*v.pw, (1-n.r.R)*v.pw
				ny0, ny1 := n.r.T*v.ph, (1-n.r.B)*v.ph
				if over(y0, y1, ny0, ny1) { // beside
					if nx1 <= x0 {
						rm.lo = math.Max(rm.lo, (nx1+x0)/2)
					}
					if nx0 >= x1 {
						rm.hi = math.Min(rm.hi, (x1+nx0)/2)
					}
				}
				if over(x0, x1, nx0, nx1) { // above or below
					if ny1 <= y0 {
						rm.loY = math.Max(rm.loY, (ny1+y0)/2)
					}
					if ny0 >= y1 {
						rm.hiY = math.Min(rm.hiY, (y1+ny0)/2)
					}
				}
			}
			k := ref{p, i}
			rooms[k] = rm
			if rm.hi-rm.lo > 1.05*(rm.hiY-rm.loY) {
				landscape[k] = 1
			}
			sz := &size[landscape[k]]
			sz[0] = math.Max(sz[0], x1-x0)
			sz[1] = math.Max(sz[1], y1-y0)
		}
	}
	groups := map[[4]int][]ref{}
	for p, vs := range views {
		for i, v := range vs {
			side := 0 // a portrait page is one page; a landscape sheet has a left and a right one
			if v.pw > 1.1*v.ph && (v.r.L+1-v.r.R)/2 >= 0.5 {
				side = 1
			}
			lower := 0 // the bottom part of a page split into rows
			for j, n := range vs {
				if j != i && over(v.r.L, 1-v.r.R, n.r.L, 1-n.r.R) && 1-n.r.B <= v.r.T+1e-9 {
					lower = 1
				}
			}
			k := [4]int{int(math.Round(v.pw / 10)), int(math.Round(v.ph / 10)), side, lower}
			groups[k] = append(groups[k], ref{p, i})
		}
	}
	for _, g := range groups {
		var tops, bots []float64
		for _, r := range g {
			v := views[r.page][r.idx]
			tops, bots = append(tops, v.r.T), append(bots, v.r.B)
		}
		medT, medB := median(tops), median(bots)
		for _, r := range g {
			v := views[r.page][r.idx]
			x0, x1 := v.r.L*v.pw, (1-v.r.R)*v.pw
			y0, y1 := v.r.T*v.ph, (1-v.r.B)*v.ph
			rm := rooms[r]
			tw, th := size[landscape[r]][0], size[landscape[r]][1]
			bx0, bx1, vx0, vx1 := place((x0+x1)/2-tw/2, tw, rm.lo, rm.hi)
			var ny0 float64
			switch {
			case math.Abs(v.r.T-medT) <= 0.02:
				ny0 = y0
			case math.Abs(v.r.B-medB) <= 0.02:
				ny0 = y1 - th
			default:
				ny0 = (y0+y1)/2 - th/2
			}
			by0, by1, vy0, vy1 := place(ny0, th, rm.loY, rm.hiY)
			// what shows never shrinks below the rectangle as detected or drawn
			vx0, vx1 = math.Min(vx0, x0), math.Max(vx1, x1)
			vy0, vy1 = math.Min(vy0, y0), math.Max(vy1, y1)
			nr := geom.Ratios{L: vx0 / v.pw, R: 1 - vx1/v.pw, T: vy0 / v.ph, B: 1 - vy1/v.ph}
			if !nr.Valid() {
				continue
			}
			o := &outs[r.page][r.idx]
			o.Rect = nr.Turn(360 - o.Turn) // back on the page as stored
			if bx0 < vx0-1e-6 || bx1 > vx1+1e-6 || by0 < vy0-1e-6 || by1 > vy1+1e-6 {
				box := geom.Ratios{L: bx0 / v.pw, R: 1 - bx1/v.pw, T: by0 / v.ph, B: 1 - by1/v.ph}.Turn(360 - o.Turn)
				o.Box = &box
			}
		}
	}
}

// place puts an interval of length n starting near a inside the room
// [lo, hi]. It returns the interval (b0, b1) and the part of it that lies in
// the room (v0, v1): all of it when it fits; otherwise the interval covers the
// whole room, as close to a as it can, and reaches past it.
func place(a, n, lo, hi float64) (b0, b1, v0, v1 float64) {
	if n <= hi-lo {
		a = math.Max(lo, math.Min(a, hi-n))
		return a, a + n, a, a + n
	}
	a = math.Max(hi-n, math.Min(a, lo))
	return a, a + n, lo, hi
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[len(s)/2]
}

// ocrDPI is the resolution pages are rendered at for recognition.
const ocrDPI = 300

// AddOCR recognises the words of every scanned page that has outputs and no
// text yet, and attaches each word to the output page it falls on. It
// returns how many pages were recognised and warnings (Tesseract missing,
// pages that failed); a missing Tesseract is a warning, not an error.
func AddOCR(ctx context.Context, eng *pdf.Engine, doc *pdf.Document, outs map[int][]crop.Output, lang string, progress Progress) (int, []string, error) {
	bin, err := ocr.Find()
	if err != nil {
		return 0, []string{err.Error()}, nil
	}
	var idx []int
	for p := range outs {
		idx = append(idx, p-1)
	}
	sort.Ints(idx)
	kinds, err := eng.PageKinds(ctx, doc, idx)
	if err != nil {
		return 0, nil, err
	}
	var jobs []pdf.RenderJob
	for _, k := range kinds {
		if !k.Scanned() || k.TextObjects > 0 {
			continue // born-digital, or already has a text layer
		}
		pi := doc.Pages[k.Page]
		w := min(4000, int(math.Round(pi.Width/72*ocrDPI)))
		jobs = append(jobs, pdf.RenderJob{Page: k.Page, Width: w, Height: max(1, int(math.Round(float64(w)*pi.Height/pi.Width)))})
	}
	if len(jobs) == 0 {
		return 0, nil, nil
	}
	var (
		mu    sync.Mutex
		warn  []string
		done  atomic.Int32
		total = len(jobs)
	)
	if progress != nil {
		progress("ocr", 0, total)
	}
	err = eng.Render(ctx, doc, jobs, func(j pdf.RenderJob, g *image.Gray) error {
		mu.Lock()
		turn := 0
		if os := outs[j.Page+1]; len(os) > 0 {
			turn = os[0].Turn // the outputs of a page share its turn
		}
		mu.Unlock()
		up := g
		if turn != 0 {
			up = raster.RotateQuarter(g, turn) // Tesseract reads upright text only
		}
		uw, uh := up.Bounds().Dx(), up.Bounds().Dy()
		words, err := ocr.Recognize(ctx, bin, up, lang)
		n := int(done.Add(1))
		if progress != nil {
			progress("ocr", n, total)
		}
		if err != nil {
			mu.Lock()
			warn = append(warn, fmt.Sprintf("page %d: %v", j.Page+1, err))
			mu.Unlock()
			return nil
		}
		pi := doc.Pages[j.Page]
		mu.Lock()
		defer mu.Unlock()
		os := outs[j.Page+1]
		for _, w := range words {
			// the box on the upright page, which is how the text layer is laid
			box := geom.Ratios{L: float64(w.X0) / float64(uw), R: 1 - float64(w.X1)/float64(uw), T: float64(w.Y0) / float64(uh), B: 1 - float64(w.Y1)/float64(uh)}
			// and the word's centre on the page as stored, to find its output
			fx, fy := geom.TurnPoint(float64(w.X0+w.X1)/2/float64(uw), float64(w.Y0+w.Y1)/2/float64(uh), 360-turn)
			if i := outputAt(os, fx*pi.Width, fy*pi.Height, pi); i >= 0 {
				os[i].Words = append(os[i].Words, crop.Word{Text: w.Text, Box: box})
			}
		}
		return nil
	})
	if err != nil {
		return 0, warn, err
	}
	return total, warn, nil
}

// outputAt returns the output whose (straightened) rectangle holds the point
// (x, y), in display points of the source page, once the point is turned
// with its output's content; -1 if none.
func outputAt(os []crop.Output, x, y float64, pi pdf.PageInfo) int {
	for i, o := range os {
		x0, x1 := o.Rect.L*pi.Width, (1-o.Rect.R)*pi.Width
		y0, y1 := o.Rect.T*pi.Height, (1-o.Rect.B)*pi.Height
		px, py := x, y
		if o.Angle != 0 {
			rx, ry := o.PivotPoint()
			cx, cy := rx*pi.Width, ry*pi.Height
			s, c := math.Sincos(o.Angle * math.Pi / 180)
			dx, dy := x-cx, y-cy
			px, py = cx+dx*c+dy*s, cy-dx*s+dy*c // counterclockwise on screen, y down
		}
		if px >= x0 && px <= x1 && py >= y0 && py <= y1 {
			return i
		}
	}
	return -1
}
