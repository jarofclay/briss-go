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

// normalize gives the output pages of each kind (same source page size, and
// left or right half of a landscape sheet) the size of the largest of them, so
// an e-reader shows every page at the same scale. Each rectangle grows about
// its centre horizontally, never into a neighbouring rectangle of the same
// sheet; vertically it keeps its top when its top is where most pages' top
// is (a short last page of a chapter), its bottom likewise (a chapter
// opening), and grows about its centre otherwise. Rectangles stay on the page.
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
	groups := map[[3]int][]ref{}
	for p, vs := range views {
		for i, v := range vs {
			side := 0 // a portrait page is one page; a landscape sheet has a left and a right one
			if v.pw > 1.1*v.ph && (v.r.L+1-v.r.R)/2 >= 0.5 {
				side = 1
			}
			k := [3]int{int(math.Round(v.pw / 10)), int(math.Round(v.ph / 10)), side}
			groups[k] = append(groups[k], ref{p, i})
		}
	}
	for _, g := range groups {
		var tw, th float64
		var tops, bots []float64
		for _, r := range g {
			v := views[r.page][r.idx]
			tw = math.Max(tw, (1-v.r.L-v.r.R)*v.pw)
			th = math.Max(th, (1-v.r.T-v.r.B)*v.ph)
			tops, bots = append(tops, v.r.T), append(bots, v.r.B)
		}
		medT, medB := median(tops), median(bots)
		for _, r := range g {
			vs := views[r.page]
			v := vs[r.idx]
			x0, x1 := v.r.L*v.pw, (1-v.r.R)*v.pw
			y0, y1 := v.r.T*v.ph, (1-v.r.B)*v.ph
			lo, hi := 0.0, v.pw // room: half way to the neighbours on the sheet
			for j, n := range vs {
				if j == r.idx {
					continue
				}
				nx0, nx1 := n.r.L*v.pw, (1-n.r.R)*v.pw
				if nx1 <= x0 {
					lo = math.Max(lo, (nx1+x0)/2)
				}
				if nx0 >= x1 {
					hi = math.Min(hi, (x1+nx0)/2)
				}
			}
			nx0, nx1 := fit((x0+x1)/2-tw/2, tw, lo, hi)
			var ny0 float64
			switch {
			case math.Abs(v.r.T-medT) <= 0.02:
				ny0 = y0
			case math.Abs(v.r.B-medB) <= 0.02:
				ny0 = y1 - th
			default:
				ny0 = (y0+y1)/2 - th/2
			}
			ny0, ny1 := fit(ny0, th, 0, v.ph)
			nr := geom.Ratios{L: nx0 / v.pw, R: 1 - nx1/v.pw, T: ny0 / v.ph, B: 1 - ny1/v.ph}
			if nr.Valid() {
				o := &outs[r.page][r.idx]
				o.Rect = nr.Turn(360 - o.Turn) // back on the page as stored
			}
		}
	}
}

// fit places an interval of length n starting near a inside [lo, hi],
// shrinking it only when it cannot fit.
func fit(a, n, lo, hi float64) (float64, float64) {
	if n >= hi-lo {
		return lo, hi
	}
	a = math.Max(lo, math.Min(a, hi-n))
	return a, a + n
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
