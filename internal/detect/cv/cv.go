// Package cv is detection tier T2: classical image analysis for scanned
// pages, run on every page on its own (PLAN.md section 8.2).
//
// For each rendered page it builds an ink mask with an Otsu threshold, keeps
// connected ink components that look like page content (dropping specks,
// anything touching the image border such as scanner edges, black bars and
// gutter shadows, and tall thin lines such as book edges), splits a landscape
// sheet into two pages at the gutter when it holds an open book, and grows
// each page's content block from its densest band so stray marks far from the
// text are left out. Running headers, footers and page numbers are kept when
// Options.KeepHeaders is set and dropped otherwise when they stand apart from
// the body.
package cv

import (
	"context"
	"fmt"
	"image"
	"math"
	"sort"
	"strings"

	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/jarofclay/briss-go/internal/raster"
)

// Name is the detector name used in crop definitions.
const Name = "cv@1"

// Parameters, as fractions of the page size unless noted. They were set on
// synthetic scans and on photocopied two-up book scans.
type Params struct {
	Width           int     // render width in pixels
	SpeckArea       int     // components smaller than this many pixels are noise
	EdgeDark        float64 // a border column or row darker than this share is scanner edge
	EdgeMax         float64 // trim at most this share of the width or height as scanner edge
	BorderLarge     float64 // components touching the border and larger than this share of a side are edge junk
	LineHeight      float64 // drop components taller than this fraction of the page and ...
	LineWidth       float64 // ... narrower than this fraction of the width (book edges, gutter lines)
	BorderDensity   float64 // components touching the border with more ink than this share of their box are edge junk
	VerticalGap     float64 // row bands closer than this merge into one band
	HorizontalGap   float64 // column bands closer than this (fraction of the page part) merge
	MinBandInk      float64 // bands with less than this share of the part's ink are noise ...
	MinBandComps    int     // ... unless they hold at least this many components (a short heading)
	NearGap         float64 // bands this close to the core block join it (page numbers, short headings)
	TwoUpAspect     float64 // sheets wider than this times their height may hold two pages
	MinHalfInk      float64 // each half needs this share of the ink ...
	MinHalfComps    int     // ... and this many components to count as a page
	HeaderBand      float64 // a top or bottom line thinner than this is a header or footer candidate
	HeaderGap       float64 // ... when it is separated from the body by at least this much
	Pad             float64 // padding added around the block, fraction of the page side
	StripeWidth     float64 // fragments narrower than this share of the width ...
	StripeCover     float64 // ... stacked in one column over more than this share of the height are a book edge
	LineJoin        float64 // glyphs closer than this many glyph heights on one line join into a text line
	MinLineGlyphs   int     // a text line has at least this many glyphs ...
	GlyphMin        float64 // ... whose median height is at least this share of the page's glyph height
	GutterShade     float64 // a binding shadow is this many grey levels darker than the other gutter candidates
	ClusterGap      float64 // text lines closer than this vertically form one cluster
	MinCluster      float64 // clusters with less than this share of the main cluster's ink ...
	MinClusterN     int     // ... and fewer glyphs than this are stray marks ...
	EdgeZone        float64 // ... when they reach into this share of the sheet height at the top or bottom
	SideGap         float64 // glyph columns this close beside the block join it (page numbers in a contents list)
	FooterWidth     float64 // a detached bottom line at least this share of the block width is body text, not a footer
	MaxSkew         float64 // straightening angles are searched up to this many degrees either way ...
	SkewContrast    float64 // ... and kept only when the best angle's baseline sharpness is this many times the average
	OrientMinGlyphs int     // a page needs this many glyphs before its orientation is judged
	OrientTile      float64 // a tile votes for a text direction when one profile varies this many times more than the other
	OrientVote      float64 // a direction wins with this many times the other's votes
	OrientUpright   float64 // ascender minus descender share needed to call text upright (or, negative, upside down)
	StackGap        float64 // two pages one above the other are separated by at least this share of the height ...
	StackAlign      float64 // ... and sit in their halves alike, within this share of the sheet
	ShadowDelta     int     // marks whose surroundings are this many grey levels darker than the paper sit in a shadow
	FlatMin         int     // columns whose paper is darker than this are left alone by flattening (black borders) ...
	FlatWidth       float64 // ... and so are shaded bands narrower than this share of the width
}

// DefaultParams are the parameters used by the detector.
var DefaultParams = Params{
	Width: 1400, SpeckArea: 8, EdgeDark: 0.8, EdgeMax: 0.12, BorderLarge: 0.05,
	LineHeight: 0.08, LineWidth: 0.012, BorderDensity: 0.3,
	VerticalGap: 0.02, HorizontalGap: 0.03,
	MinBandInk: 0.01, MinBandComps: 3, NearGap: 0.05,
	TwoUpAspect: 1.15, MinHalfInk: 0.02, MinHalfComps: 6,
	HeaderBand: 0.022, HeaderGap: 0.012,
	Pad: 0.003, StripeWidth: 0.006, StripeCover: 0.2,
	LineJoin: 1.2, MinLineGlyphs: 3, GlyphMin: 0.5, GutterShade: 10,
	ClusterGap: 0.06, MinCluster: 0.02, MinClusterN: 8, EdgeZone: 0.05, SideGap: 0.3, FooterWidth: 0.6, MaxSkew: 5, SkewContrast: 1.6, ShadowDelta: 30,
	OrientMinGlyphs: 120, OrientTile: 1.4, OrientVote: 2, OrientUpright: 0.2, StackGap: 0.06, StackAlign: 0.03, FlatMin: 70, FlatWidth: 0.08,
}

// Detector implements detect.PageDetector.
type Detector struct {
	P Params
	// set on the inner calls that work on a turned page or on one half of
	// a stacked sheet, so they do not turn or cut again
	noTurn, noRowSplit bool
}

// New returns a detector with the default parameters.
func New() Detector { return Detector{P: DefaultParams} }

// Name implements detect.Detector.
func (Detector) Name() string { return Name }

// Available implements detect.Detector; the tier needs nothing external.
func (Detector) Available(context.Context) error { return nil }

// RenderWidth implements detect.PageDetector.
func (d Detector) RenderWidth() int { return d.params().Width }

func (d Detector) params() Params {
	if d.P.Width == 0 {
		return DefaultParams
	}
	return d.P
}

// Detect implements detect.Detector for callers that only have the cluster
// overlay; the per-page path (DetectPage) is the intended one.
func (d Detector) Detect(_ context.Context, req detect.Request) (detect.Candidate, error) {
	c := detect.Candidate{Source: Name}
	if req.Overlay == nil {
		return c, nil
	}
	r := d.DetectPage(req.Overlay, req.Options)
	c.Rects, c.Confidence, c.Notes = r.Rects, r.Confidence, r.Notes
	return c, nil
}

type comp struct {
	x0, y0, x1, y1 int // inclusive pixel bounds
	area           int
}

func (c comp) cx() int { return (c.x0 + c.x1) / 2 }
func (c comp) cy() int { return (c.y0 + c.y1) / 2 }

// Trace records the detector's intermediate results for debugging.
type Trace struct {
	Threshold                  uint8
	EdgeL, EdgeT, EdgeR, EdgeB int // trimmed scanner edges, pixels
	Kept, Dropped              []comp
	Reasons                    []string // per dropped component
	Parts                      [][2]int
	Turn                       int      // clockwise quarter turn the page needs, when AutoRotate found one
	TurnSure                   bool     // the orientation was judged, not left alone
	RowSplit                   int      // row where a sheet of two pages one above the other is cut
	Lines                      []comp   // text lines, figures and marks that make up the blocks
	LineWhy                    []string // per line: core, near, beside or column
	Blocks                     [][4]int
}

// DetectPage implements detect.PageDetector.
func (d Detector) DetectPage(img *image.Gray, opts detect.Options) detect.PageResult {
	r, tr := d.trace(img, opts)
	switch {
	case tr.Turn != 0:
		// Sideways or upside down: find the pages on the page turned upright
		// (where a sideways two-page spread becomes a left and a right page),
		// then measure the rectangles on the page as it is.
		inner := d
		inner.noTurn = true
		upImg := raster.RotateQuarter(img, tr.Turn)
		// A sideways page turned upright is wider than pages are rendered;
		// bring it to the usual width so pixel-sized thresholds mean the same.
		if uw, uh := upImg.Bounds().Dx(), upImg.Bounds().Dy(); uw > d.RenderWidth() {
			upImg = raster.Resize(upImg, d.RenderWidth(), max(1, uh*d.RenderWidth()/uw))
		}
		up := inner.DetectPage(upImg, opts)
		out := detect.PageResult{Angles: up.Angles, Confidence: up.Confidence, TurnSure: tr.TurnSure}
		for range up.Rects {
			out.Turns = append(out.Turns, tr.Turn)
		}
		for _, rr := range up.Rects {
			out.Rects = append(out.Rects, rr.Turn(360-tr.Turn))
		}
		out.Notes = strings.TrimSuffix(fmt.Sprintf("turned %d°; %s", tr.Turn, up.Notes), "; ")
		return out
	case tr.RowSplit > 0:
		r = d.stacked(img, tr.RowSplit, opts)
	}
	r.TurnSure = tr.TurnSure
	return r
}

// stacked detects the two pages of a sheet cut at row split, top first.
func (d Detector) stacked(img *image.Gray, split int, opts detect.Options) detect.PageResult {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	inner := d
	inner.noTurn, inner.noRowSplit = true, true
	out := detect.PageResult{Confidence: 1, Notes: "two pages, one above the other"}
	for _, half := range [][2]int{{0, split}, {split, h}} {
		hh := half[1] - half[0]
		r := inner.DetectPage(raster.Crop(img, image.Rect(b.Min.X, b.Min.Y+half[0], b.Min.X+w, b.Min.Y+half[1])), opts)
		for i, rr := range r.Rects {
			top := float64(half[0]) + rr.T*float64(hh)
			bot := float64(half[0]) + (1-rr.B)*float64(hh)
			out.Rects = append(out.Rects, geom.Ratios{L: rr.L, R: rr.R, T: top / float64(h), B: 1 - bot/float64(h)})
			a := 0.0
			if i < len(r.Angles) {
				a = r.Angles[i]
			}
			out.Angles = append(out.Angles, a)
		}
		out.Confidence = math.Min(out.Confidence, r.Confidence)
	}
	return out
}

func (d Detector) trace(img *image.Gray, opts detect.Options) (detect.PageResult, *Trace) {
	p := d.params()
	tr := &Trace{}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 20 || h < 20 {
		return detect.PageResult{Rects: []geom.Ratios{{}}, Notes: "page image too small"}, tr
	}
	// The ink mask comes from a copy with column shading (binding shadows)
	// flattened; the shadow and paper tests below look at the page as it is.
	flat := flatten(img, p)
	thr := otsu(flat)
	tr.Threshold = thr
	mask := make([]bool, w*h)
	for y := 0; y < h; y++ {
		row := flat.Pix[y*flat.Stride:]
		for x := 0; x < w; x++ {
			mask[y*w+x] = row[x] < thr
		}
	}
	// Scanner edges: full-length dark columns and rows at the borders.
	l, t, rr, bt := edges(mask, w, h, p.EdgeDark, p.EdgeMax)
	tr.EdgeL, tr.EdgeT, tr.EdgeR, tr.EdgeB = l, t, rr, bt
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < l || x >= w-rr || y < t || y >= h-bt {
				mask[y*w+x] = false
			}
		}
	}
	x0b, y0b, x1b, y1b := l, t, w-1-rr, h-1-bt // usable area after trimming
	var kept []comp
	for _, c := range components(mask, w, h) {
		cw, ch := c.x1-c.x0+1, c.y1-c.y0+1
		density := float64(c.area) / float64(cw*ch)
		touches := c.x0 <= x0b+1 || c.y0 <= y0b+1 || c.x1 >= x1b-1 || c.y1 >= y1b-1
		reason := ""
		switch {
		case c.area < p.SpeckArea:
			reason = "speck"
		case float64(ch) > p.LineHeight*float64(h) && float64(cw) < p.LineWidth*float64(w):
			reason = "thin line"
		case touches && (density > p.BorderDensity || float64(cw) > p.BorderLarge*float64(w) || float64(ch) > p.BorderLarge*float64(h)):
			reason = "edge junk"
		case touches && float64(cw) < p.LineWidth*float64(w)*2:
			reason = "edge sliver"
		case float64(cw) > 0.9*float64(w) || float64(ch) > 0.9*float64(h):
			reason = "spans the page"
		}
		if reason != "" {
			tr.Dropped = append(tr.Dropped, c)
			tr.Reasons = append(tr.Reasons, reason)
			continue
		}
		kept = append(kept, c)
	}
	// Glyphs cut by the edge of the scan (a running header or a page number
	// half outside the image) touch the border like scanner junk does; give
	// the glyph-sized ones back, they still have to line up as text to count.
	if hs := heights(kept); len(hs) > 0 {
		mh := hs[len(hs)/2]
		keptDropped, keptReasons := tr.Dropped[:0], tr.Reasons[:0]
		for i, c := range tr.Dropped {
			cw, ch := c.x1-c.x0+1, c.y1-c.y0+1
			if tr.Reasons[i] == "edge junk" && cw <= 2*mh && ch <= 2*mh {
				kept = append(kept, c)
				continue
			}
			keptDropped, keptReasons = append(keptDropped, c), append(keptReasons, tr.Reasons[i])
		}
		tr.Dropped, tr.Reasons = keptDropped, keptReasons
	}
	kept, stripes := dropStripes(kept, w, h, p)
	for _, c := range stripes {
		tr.Dropped = append(tr.Dropped, c)
		tr.Reasons = append(tr.Reasons, "edge stripe")
	}
	tr.Kept = kept
	if len(kept) == 0 {
		return detect.PageResult{Rects: []geom.Ratios{{}}, Notes: "no content found; keeping the whole page"}, tr
	}
	if !d.noTurn && opts.ForceTurn%360 != 0 {
		tr.Turn = ((opts.ForceTurn % 360) + 360) % 360
		return detect.PageResult{}, tr
	}
	if opts.AutoRotate && !d.noTurn {
		turn, conf := orientation(mask, w, h, kept, p)
		tr.TurnSure = conf > 0
		if turn != 0 {
			tr.Turn = turn
			return detect.PageResult{}, tr
		}
	}
	parts := [][2]int{{0, w}}
	notes := ""
	if !opts.NoAutoSplit && float64(w) > p.TwoUpAspect*float64(h) {
		if split, ok := gutter(kept, img, t, h-bt, p); ok {
			parts = [][2]int{{0, split}, {split, w}}
			notes = "two-page sheet"
		}
	}
	if len(parts) == 1 && !d.noRowSplit && !opts.NoAutoSplit && float64(h) > p.TwoUpAspect*float64(w) {
		if split, ok := rowGutter(kept, w, h, t, h-bt, p); ok {
			tr.RowSplit = split
			return detect.PageResult{}, tr
		}
	}
	tr.Parts = parts
	total := 0
	for _, c := range kept {
		total += c.area
	}
	onPaper := paperTest(img, otsu(img), p)
	var rects []geom.Ratios
	var angles []float64
	conf := 1.0
	for _, part := range parts {
		var in []comp
		ink := 0
		for _, c := range kept {
			if c.cx() >= part[0] && c.cx() < part[1] {
				in = append(in, c)
				ink += c.area
			}
		}
		if len(parts) == 2 && (float64(ink) < p.MinHalfInk*float64(total) || len(in) < p.MinHalfComps) {
			continue // blank side of a spread
		}
		bx0, by0, bx1, by1, ang, ok := lineBlock(in, part, [2]int{t, h - bt}, w, h, opts.KeepHeaders, p, tr, onPaper)
		if !ok {
			continue
		}
		tr.Blocks = append(tr.Blocks, [4]int{bx0, by0, bx1, by1})
		px, py := int(p.Pad*float64(w)), int(p.Pad*float64(h))
		for _, b := range splitBlock([4]int{bx0, by0, bx1, by1}, in, opts) {
			x0, y0 := max(0, b[0]-px), max(0, b[1]-py)
			x1, y1 := min(w, b[2]+1+px), min(h, b[3]+1+py)
			rects = append(rects, geom.FromPixels(float64(x0), float64(y0), float64(x1-x0), float64(y1-y0), w, h))
			angles = append(angles, ang)
		}
		conf = math.Min(conf, float64(ink)/float64(total)*float64(len(parts)))
	}
	if len(rects) == 0 {
		return detect.PageResult{Rects: []geom.Ratios{{}}, Notes: "no content block found; keeping the whole page"}, tr
	}
	if len(parts) == 2 && len(rects) == 1 {
		notes = "two-page sheet with one blank side"
	}
	return detect.PageResult{Rects: rects, Angles: angles, Confidence: math.Min(1, conf), Notes: notes}, tr
}

// dropStripes removes book-edge stripes: narrow fragments (taller than wide)
// that stack up in one column over a large part of the page height, as the
// edges of the page block do on a photocopied book. Single tall letters in a
// text column never cover that much height in one pixel column.
func dropStripes(cs []comp, w, h int, p Params) (kept, dropped []comp) {
	narrow := func(c comp) bool {
		cw, ch := c.x1-c.x0+1, c.y1-c.y0+1
		return float64(cw) <= p.StripeWidth*float64(w) && ch >= 2*cw
	}
	cover := make([]int, w)
	for _, c := range cs {
		if narrow(c) {
			for x := c.x0; x <= c.x1; x++ {
				cover[x] += c.y1 - c.y0 + 1
			}
		}
	}
	limit := int(p.StripeCover * float64(h))
	stripe := make([]bool, w)
	for x := range cover {
		if cover[x] > limit {
			for k := max(0, x-2); k <= min(w-1, x+2); k++ {
				stripe[k] = true
			}
		}
	}
	for _, c := range cs {
		hit := false
		if float64(c.x1-c.x0+1) <= 2*p.StripeWidth*float64(w) {
			for x := c.x0; x <= c.x1 && !hit; x++ {
				hit = stripe[x]
			}
		}
		if hit {
			dropped = append(dropped, c)
		} else {
			kept = append(kept, c)
		}
	}
	return kept, dropped
}

// edges returns how many columns or rows at each border are scanner edge:
// consecutive lines from the border whose dark share exceeds limit.
func edges(mask []bool, w, h int, limit, maxShare float64) (l, t, r, b int) {
	col := func(x int) float64 {
		n := 0
		for y := 0; y < h; y++ {
			if mask[y*w+x] {
				n++
			}
		}
		return float64(n) / float64(h)
	}
	row := func(y int) float64 {
		n := 0
		for x := 0; x < w; x++ {
			if mask[y*w+x] {
				n++
			}
		}
		return float64(n) / float64(w)
	}
	mw, mh := int(maxShare*float64(w)), int(maxShare*float64(h))
	for l < mw && col(l) > limit {
		l++
	}
	for r < mw && col(w-1-r) > limit {
		r++
	}
	for t < mh && row(t) > limit {
		t++
	}
	for b < mh && row(h-1-b) > limit {
		b++
	}
	return l, t, r, b
}

// gutter finds the split column of an open-book spread: a run of nearly
// empty columns in the middle 30% of the sheet with content on both sides.
// When several runs qualify (a table of contents leaves one between its
// entries and their page numbers) it takes the one holding the darkest
// column, the binding shadow, if that is clearly darker than the rest, and
// the widest run otherwise.
func gutter(cs []comp, img *image.Gray, y0, y1 int, p Params) (int, bool) {
	b := img.Bounds()
	w := b.Dx()
	prof := make([]int, w)
	for _, c := range cs {
		per := max(1, c.area/(c.x1-c.x0+1))
		for x := c.x0; x <= c.x1; x++ {
			prof[x] += per
		}
	}
	lo, hi := int(0.35*float64(w)), int(0.65*float64(w))
	peak := 0
	for _, v := range prof {
		peak = max(peak, v)
	}
	if peak == 0 {
		return 0, false
	}
	limit := max(1, peak/50)
	minLen := max(3, w/200)
	type run struct{ a, b int } // [a, b)
	var runs []run
	start := -1
	for x := lo; x <= hi; x++ {
		empty := x < hi && prof[x] <= limit
		if empty && start < 0 {
			start = x
		}
		if !empty && start >= 0 {
			if x-start >= minLen {
				runs = append(runs, run{start, x})
			}
			start = -1
		}
	}
	if len(runs) == 0 {
		return 0, false
	}
	best := 0
	for i, r := range runs {
		if r.b-r.a > runs[best].b-runs[best].a {
			best = i
		}
	}
	if len(runs) > 1 && y1 > y0 {
		// Median grey level per column (text on some rows does not move it,
		// a shadow running down the whole binding does), taken a little past
		// each run's ends because the darkest core of a shadow can hold ink.
		med := make([]float64, w)
		var hist [256]int
		for x := lo; x < hi; x++ {
			hist = [256]int{}
			for y := y0; y < y1; y++ {
				hist[img.Pix[(b.Min.Y+y)*img.Stride+b.Min.X+x]]++
			}
			half, acc := (y1-y0)/2, 0
			for v, n := range hist {
				if acc += n; acc > half {
					med[x] = float64(v)
					break
				}
			}
		}
		ext := max(3, w/100)
		dark := make([]float64, len(runs))
		for i, r := range runs {
			dark[i] = math.MaxFloat64
			a, bb := max(lo, r.a-ext), min(hi, r.b+ext)
			for x := a; x < bb; x++ {
				sum, n := 0.0, 0
				for k := max(a, x-2); k <= min(bb-1, x+2); k++ {
					sum += med[k]
					n++
				}
				dark[i] = math.Min(dark[i], sum/float64(n))
			}
		}
		di, light := 0, 0.0
		for i := range runs {
			if dark[i] < dark[di] {
				di = i
			}
			light = math.Max(light, dark[i])
		}
		if light-dark[di] >= p.GutterShade {
			best = di
		}
	}
	r := runs[best]
	return (r.a + r.b) / 2, true
}

type band struct{ a, b, ink, n int } // [a, b), ink pixels, components

// bands merges a 1-D profile into runs of non-zero entries separated by gaps
// shorter than gap.
func bands(prof []int, gap int) []band {
	var out []band
	start, last := -1, -1
	ink := 0
	for i, v := range prof {
		if v == 0 {
			continue
		}
		if start >= 0 && i-last-1 >= gap {
			out = append(out, band{a: start, b: last + 1, ink: ink})
			start, ink = -1, 0
		}
		if start < 0 {
			start = i
		}
		last = i
		ink += v
	}
	if start >= 0 {
		out = append(out, band{a: start, b: last + 1, ink: ink})
	}
	return out
}

// significant returns the outer extent of the core bands (those holding
// enough ink or enough components) grown by every band within near of that
// extent, so a lone page number or a short heading next to the text joins the
// block while marks far from it do not. centers counts component centres per
// profile index.
func significant(bs []band, centers []int, total, minComps int, minInk float64, near int) (int, int, bool) {
	in := make([]bool, len(bs))
	a, b := -1, -1
	for i, bd := range bs {
		n := 0
		for k := bd.a; k < bd.b; k++ {
			n += centers[k]
		}
		if float64(bd.ink) < minInk*float64(total) && n < minComps {
			continue
		}
		in[i] = true
		if a < 0 || bd.a < a {
			a = bd.a
		}
		b = max(b, bd.b)
	}
	if a < 0 {
		return 0, 0, false
	}
	for grown := true; grown; {
		grown = false
		for i, bd := range bs {
			if in[i] {
				continue
			}
			if (bd.a >= b && bd.a-b <= near) || (bd.b <= a && a-bd.b <= near) || (bd.a < b && bd.b > a) {
				in[i] = true
				a, b = min(a, bd.a), max(b, bd.b)
				grown = true
			}
		}
	}
	return a, b, true
}

// block finds the content block of one page part: the extent of all row bands
// and then all column bands that carry a real share of the ink, so headings
// and short lines stay in while isolated specks and marks fall out;
// optionally without a detached header or footer line.
func block(cs []comp, part [2]int, w, h int, keepHeaders bool, p Params) (x0, y0, x1, y1 int, ok bool) {
	if len(cs) == 0 {
		return 0, 0, 0, 0, false
	}
	rows := make([]int, h)
	rowCenters := make([]int, h)
	total := 0
	for _, c := range cs {
		per := max(1, c.area/(c.y1-c.y0+1))
		for y := c.y0; y <= c.y1; y++ {
			rows[y] += per
		}
		rowCenters[c.cy()]++
		total += c.area
	}
	ya, yb, ok := significant(bands(rows, max(2, int(p.VerticalGap*float64(h)))), rowCenters, total, p.MinBandComps, p.MinBandInk, int(p.NearGap*float64(h)))
	if !ok {
		return 0, 0, 0, 0, false
	}
	y0, y1 = ya, yb-1
	if !keepHeaders {
		y0, y1 = trimHeaderFooter(rows, y0, y1, h, p)
	}
	cols := make([]int, w)
	colCenters := make([]int, w)
	totalIn := 0
	for _, c := range cs {
		if c.cy() < y0 || c.cy() > y1 {
			continue
		}
		per := max(1, c.area/(c.x1-c.x0+1))
		for x := c.x0; x <= c.x1; x++ {
			cols[x] += per
		}
		colCenters[c.cx()]++
		totalIn += c.area
	}
	pw := part[1] - part[0]
	xa, xb, ok := significant(bands(cols[part[0]:part[1]], max(2, int(p.HorizontalGap*float64(pw)))), colCenters[part[0]:part[1]], totalIn, p.MinBandComps, p.MinBandInk, int(p.NearGap*float64(pw)))
	if !ok {
		return 0, 0, 0, 0, false
	}
	bx0, bx1 := part[0]+xa, part[0]+xb-1
	x0, y0b, x1, y1b := math.MaxInt, math.MaxInt, -1, -1
	for _, c := range cs {
		if c.cy() < y0 || c.cy() > y1 || c.cx() < bx0 || c.cx() > bx1 {
			continue
		}
		x0, y0b = min(x0, c.x0), min(y0b, c.y0)
		x1, y1b = max(x1, c.x1), max(y1b, c.y1)
	}
	if x1 < 0 {
		return 0, 0, 0, 0, false
	}
	return x0, y0b, x1, y1b, true
}

// trimHeaderFooter drops a thin line at the top or bottom of [y0, y1] that
// is separated from the rest by a clear gap: a running header, footer or
// page-number line. Large headings are thicker than HeaderBand and stay.
func trimHeaderFooter(rows []int, y0, y1, h int, p Params) (int, int) {
	inner := bands(rows[y0:y1+1], 2) // offsets relative to y0
	if len(inner) < 3 {
		return y0, y1
	}
	thin := int(p.HeaderBand * float64(h))
	gap := int(p.HeaderGap * float64(h))
	n := len(inner)
	ny0, ny1 := y0+inner[0].a, y0+inner[n-1].b-1
	if f, s := inner[0], inner[1]; f.b-f.a <= thin && s.a-f.b >= gap {
		ny0 = y0 + s.a
	}
	if l, s := inner[n-1], inner[n-2]; l.b-l.a <= thin && l.a-s.b >= gap {
		ny1 = y0 + s.b - 1
	}
	return ny0, ny1
}

func otsu(img *image.Gray) uint8 {
	var hist [256]int
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for _, v := range img.Pix[y*img.Stride+b.Min.X : y*img.Stride+b.Max.X] {
			hist[v]++
		}
	}
	total := b.Dx() * b.Dy()
	var sum float64
	for i, c := range hist {
		sum += float64(i * c)
	}
	var sumB, wB float64
	best, thr := -1.0, 128
	for t := 0; t < 256; t++ {
		wB += float64(hist[t])
		if wB == 0 {
			continue
		}
		wF := float64(total) - wB
		if wF == 0 {
			break
		}
		sumB += float64(t * hist[t])
		mB, mF := sumB/wB, (sum-sumB)/wF
		if v := wB * wF * (mB - mF) * (mB - mF); v > best {
			best, thr = v, t
		}
	}
	// keep light background texture out of the mask on very clean pages
	return uint8(min(thr+1, 200))
}

// components labels 8-connected true pixels and returns their bounding boxes.
func components(mask []bool, w, h int) []comp {
	parent := make([]int32, 0, 1024)
	find := func(i int32) int32 {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int32) {
		ra, rb := find(a), find(b)
		if ra != rb {
			if ra < rb {
				parent[rb] = ra
			} else {
				parent[ra] = rb
			}
		}
	}
	labels := make([]int32, w*h)
	for i := range labels {
		labels[i] = -1
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if !mask[i] {
				continue
			}
			l := int32(-1)
			for _, nb := range [4][2]int{{-1, 0}, {-1, -1}, {0, -1}, {1, -1}} {
				nx, ny := x+nb[0], y+nb[1]
				if nx < 0 || ny < 0 || nx >= w {
					continue
				}
				if nl := labels[ny*w+nx]; nl >= 0 {
					if l < 0 {
						l = nl
					} else if nl != l {
						union(l, nl)
					}
				}
			}
			if l < 0 {
				l = int32(len(parent))
				parent = append(parent, l)
			}
			labels[i] = l
		}
	}
	idx := map[int32]int{}
	var out []comp
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			l := labels[y*w+x]
			if l < 0 {
				continue
			}
			r := find(l)
			k, ok := idx[r]
			if !ok {
				k = len(out)
				idx[r] = k
				out = append(out, comp{x0: x, y0: y, x1: x, y1: y})
			}
			c := &out[k]
			c.x0, c.y0 = min(c.x0, x), min(c.y0, y)
			c.x1, c.y1 = max(c.x1, x), max(c.y1, y)
			c.area++
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].area > out[j].area })
	return out
}

// group is a set of components joined into one text line (or one figure).
type group struct {
	comp
	n      int
	ink    int
	medH   int
	base   int     // baseline: median bottom of the group's marks ...
	slope  float64 // ... tilting by this much per pixel to the right (skew)
	fitted bool    // slope comes from a fit over the whole line
	items  []int
}

// baseAt is the group's baseline at column x.
func (g group) baseAt(x int) float64 {
	return float64(g.base) + g.slope*float64(x-g.cx())
}

// onBaseline reports whether g sits on o's baseline, extended to g.
func onBaseline(g, o group, mh int) bool {
	d := float64(g.base) - o.baseAt(g.cx())
	return 4*math.Abs(d) <= float64(max(8, mh))
}

// lines joins components that sit side by side on one line, close
// horizontally and overlapping vertically, into groups, and returns them
// with the page's typical glyph height.
func lines(cs []comp, p Params) ([]group, int) {
	var hs []int
	for _, c := range cs {
		if ch := c.y1 - c.y0 + 1; ch >= 3 {
			hs = append(hs, ch)
		}
	}
	if len(hs) == 0 {
		return nil, 0
	}
	sort.Ints(hs)
	mh := hs[len(hs)/2]
	order := make([]int, len(cs))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return cs[order[a]].x0 < cs[order[b]].x0 })
	parent := make([]int, len(cs))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	maxGap := int(p.LineJoin * float64(mh) * 3) // widest gap any pair may have
	for oi, i := range order {
		a := cs[i]
		ha := a.y1 - a.y0 + 1
		for _, j := range order[oi+1:] {
			b := cs[j]
			if b.x0-a.x1 > maxGap {
				break
			}
			hb := b.y1 - b.y0 + 1
			small := min(ha, hb)
			gap := max(b.x0-a.x1, a.x0-b.x1, 0)
			if float64(gap) > p.LineJoin*float64(max(mh, small)) {
				continue
			}
			if ov := min(a.y1, b.y1) - max(a.y0, b.y0) + 1; 2*ov < small {
				continue
			}
			if ra, rb := find(i), find(j); ra != rb {
				parent[rb] = ra
			}
		}
	}
	idx := map[int]int{}
	var gs []group
	for i, c := range cs {
		r := find(i)
		k, ok := idx[r]
		if !ok {
			k = len(gs)
			idx[r] = k
			gs = append(gs, group{comp: c})
		}
		g := &gs[k]
		g.x0, g.y0 = min(g.x0, c.x0), min(g.y0, c.y0)
		g.x1, g.y1 = max(g.x1, c.x1), max(g.y1, c.y1)
		g.n++
		g.ink += c.area
		g.items = append(g.items, i)
	}
	for k := range gs {
		var gh, gb []int
		for _, i := range gs[k].items {
			gh = append(gh, cs[i].y1-cs[i].y0+1)
			gb = append(gb, cs[i].y1)
		}
		sort.Ints(gh)
		sort.Ints(gb)
		gs[k].medH, gs[k].base = gh[len(gh)/2], gb[len(gb)/2]
		gs[k].slope, gs[k].fitted = skew(cs, gs[k], mh)
	}
	return gs, mh
}

// lineBlock finds the content block of one page part from its text lines:
// groups of at least MinLineGlyphs glyph-sized components side by side, and
// figures (groups at least three glyph heights tall and wide). Small groups
// of glyphs just above or below that block (page numbers, short headings)
// join it; specks, scanner dust and the fragments of page edges, which never
// line up into text, stay out. Falls back to the band method when the part
// holds no text line.
func lineBlock(cs []comp, part, usable [2]int, w, h int, keepHeaders bool, p Params, tr *Trace, onPaper func(comp, int) bool) (x0, y0, x1, y1 int, angle float64, ok bool) {
	gs, mh := lines(cs, p)
	if mh == 0 {
		x0, y0, x1, y1, ok = block(cs, part, w, h, keepHeaders, p)
		return x0, y0, x1, y1, 0, ok
	}
	glyphLike := func(g group) bool { return float64(g.medH) >= p.GlyphMin*float64(mh) }
	// bigMark reports a mark at least size long that is not a sliver
	bigMark := func(g group, size int) bool {
		for _, i := range g.items {
			c := cs[i]
			cw, ch := c.x1-c.x0+1, c.y1-c.y0+1
			if max(cw, ch) >= size && 2*min(cw, ch) >= size {
				return true
			}
		}
		return false
	}
	isCore := func(g group) bool {
		gw, gh := g.x1-g.x0+1, g.y1-g.y0+1
		// a text line is wider than tall; stacked page-edge fragments are not
		text := g.n >= p.MinLineGlyphs && gw >= 2*mh && 2*gw >= 3*gh && glyphLike(g)
		// a figure holds at least one large mark (a drawing, a photograph,
		// a big initial); a chain of specks or page-edge slivers does not
		figure := gw >= 3*mh && gh >= 3*mh && (glyphLike(g) || g.ink >= 4*mh*mh) && bigMark(g, 3*mh)
		return text || figure
	}
	in := make([]bool, len(gs))
	why := make([]string, len(gs))
	var core []int
	for k, g := range gs {
		if isCore(g) {
			core = append(core, k)
		}
	}
	// A short line standing apart from every longer one, beside the text
	// rather than above or below it and off its baselines, is a fragment (a
	// few marks on the edge of the facing page) and not part of the page.
	wide := func(g group) bool { return g.x1-g.x0+1 >= 4*mh }
	kept := core[:0]
	for _, k := range core {
		g, ok := gs[k], wide(gs[k])
		for _, j := range core {
			if ok {
				break
			}
			o := gs[j]
			if j == k || !wide(o) {
				continue
			}
			// above or below a longer line, or on its baseline (the page
			// number of a contents entry)
			ok = (o.x0 <= g.x1 && g.x0 <= o.x1) || onBaseline(g, o, mh)
		}
		if ok {
			kept = append(kept, k)
		}
	}
	core = kept
	if len(core) == 0 {
		if part[1]-part[0] < w {
			return 0, 0, 0, 0, 0, false // one side of a spread with no text: blank
		}
		x0, y0, x1, y1, ok = block(cs, part, w, h, keepHeaders, p)
		return x0, y0, x1, y1, 0, ok
	}
	// Stray marks that happen to line up (a fragment of the facing page, a
	// smudge along the border) form small clusters of their own near the top
	// or bottom of the sheet, apart from the text; drop them unless they
	// carry real ink or several glyphs. A short heading inside the page
	// margins stays.
	sort.Slice(core, func(a, b int) bool { return gs[core[a]].y0 < gs[core[b]].y0 })
	type cluster struct{ first, last, ink, n, y1 int }
	var cl []cluster
	cgap := int(p.ClusterGap * float64(h))
	for i, k := range core {
		g := gs[k]
		if len(cl) == 0 || g.y0-cl[len(cl)-1].y1 > cgap {
			cl = append(cl, cluster{first: i, y1: g.y1})
		}
		c := &cl[len(cl)-1]
		c.last, c.ink, c.n, c.y1 = i, c.ink+g.ink, c.n+g.n, max(c.y1, g.y1)
	}
	mainInk := 0
	for _, c := range cl {
		mainInk = max(mainInk, c.ink)
	}
	bx0, by0, bx1, by1 := math.MaxInt, math.MaxInt, -1, -1
	for _, c := range cl {
		zone := int(p.EdgeZone * float64(usable[1]-usable[0]))
		atEdge := gs[core[c.first]].y0 < usable[0]+zone || c.y1 >= usable[1]-zone
		if atEdge && float64(c.ink) < p.MinCluster*float64(mainInk) && c.n < p.MinClusterN {
			continue
		}
		for _, k := range core[c.first : c.last+1] {
			g := gs[k]
			in[k] = true
			why[k] = "core"
			bx0, by0 = min(bx0, g.x0), min(by0, g.y0)
			bx1, by1 = max(bx1, g.x1), max(by1, g.y1)
		}
	}
	// Candidates to join the block are made of glyph-sized, solid marks:
	// specks, dust and dot leaders are not.
	// maxH is the tallest mark allowed, in fifths of the glyph height.
	glyphShaped := func(g group, maxH int) bool {
		for _, i := range g.items {
			c := cs[i]
			cw, ch := c.x1-c.x0+1, c.y1-c.y0+1
			if 5*ch < 3*mh || 5*ch > maxH*mh || 6*cw < ch || 8*c.area < mh*mh {
				return false
			}
		}
		return true
	}
	near := int(p.NearGap * float64(h))
	for grown := true; grown; {
		grown = false
		for k, g := range gs {
			if in[k] || !glyphLike(g) || g.y1-g.y0+1 > 3*mh || !glyphShaped(g, 15) || !onPaper(g.comp, mh) {
				continue
			}
			if g.cx() < bx0 || g.cx() > bx1 {
				continue
			}
			if (g.y0 > by1 && g.y0-by1 <= near) || (g.y1 < by0 && by0-g.y1 <= near) || (g.y0 <= by1 && g.y1 >= by0) {
				in[k] = true
				why[k] = "near"
				by0, by1 = min(by0, g.y0), max(by1, g.y1)
				grown = true
			}
		}
	}
	// Glyphs beside the block join it when they sit on the baseline of a
	// text line: one close to the end of its line (a word set apart, a verse
	// number), or a column of them standing next to most of the lines they
	// span (the page numbers of a contents list). Page-edge fragments are
	// too thin, too tall or off the baselines.
	var coreLines []group
	for k, g := range gs {
		if in[k] {
			coreLines = append(coreLines, g)
		}
	}
	level := func(g, o group) bool { return 2*(min(g.y1, o.y1)-max(g.y0, o.y0)+1) >= g.y1-g.y0+1 }
	side := int(p.SideGap * float64(part[1]-part[0]))
	var left, right []int
	for k, g := range gs {
		if in[k] || !glyphLike(g) || g.y0 < by0 || g.y1 > by1 || !glyphShaped(g, 8) || !onPaper(g.comp, mh) {
			continue
		}
		onLine, close := false, false
		for _, o := range coreLines {
			if level(g, o) && onBaseline(g, o, mh) {
				onLine = true
				close = close || max(g.x0-o.x1, o.x0-g.x1) <= 2*mh
			}
		}
		switch {
		case !onLine:
		case close:
			in[k] = true
			why[k] = "beside"
		case g.x0 > bx1 && g.x0-bx1 <= side:
			right = append(right, k)
		case g.x1 < bx0 && bx0-g.x1 <= side:
			left = append(left, k)
		}
	}
	column := func(ks []int, edge func(group) int) {
		if len(ks) < 3 {
			return
		}
		es := make([]int, len(ks))
		for i, k := range ks {
			es[i] = edge(gs[k])
		}
		sort.Ints(es)
		med := es[len(es)/2]
		var hit []int
		top, bot := math.MaxInt, -1
		for _, k := range ks {
			if d := edge(gs[k]) - med; 2*max(d, -d) <= 3*mh {
				hit = append(hit, k)
				top, bot = min(top, gs[k].y0), max(bot, gs[k].y1)
			}
		}
		spanned := 0
		for _, o := range coreLines {
			if o.cy() >= top && o.cy() <= bot {
				spanned++
			}
		}
		if len(hit) < 3 || 2*len(hit) < spanned {
			return
		}
		for _, k := range hit {
			in[k] = true
			why[k] = "column"
		}
	}
	column(right, func(g group) int { return g.x1 })
	column(left, func(g group) int { return g.x0 })
	var sel []comp
	for k, g := range gs {
		if !in[k] {
			continue
		}
		if tr != nil {
			tr.Lines = append(tr.Lines, g.comp)
			tr.LineWhy = append(tr.LineWhy, why[k])
		}
		for _, i := range g.items {
			sel = append(sel, cs[i])
		}
	}
	ya, yb := by0, by1
	if !keepHeaders {
		rows := make([]int, h)
		for _, c := range sel {
			per := max(1, c.area/(c.y1-c.y0+1))
			for y := c.y0; y <= c.y1; y++ {
				rows[y] += per
			}
		}
		ya, yb = trimHeaderFooter(rows, by0, by1, h, p)
		// A footer is short (a page number, a short running foot); a full
		// line of text at the bottom is the body's last line, set apart by a
		// paragraph break, and stays.
		if yb < by1 {
			fx0, fx1 := math.MaxInt, -1
			for _, c := range sel {
				if c.cy() > yb {
					fx0, fx1 = min(fx0, c.x0), max(fx1, c.x1)
				}
			}
			if fx1 >= 0 && float64(fx1-fx0) >= p.FooterWidth*float64(bx1-bx0) {
				yb = by1
			}
		}
	}
	x0, y0, x1, y1 = math.MaxInt, math.MaxInt, -1, -1
	for _, c := range sel {
		if c.cy() < ya || c.cy() > yb {
			continue
		}
		x0, y0 = min(x0, c.x0), min(y0, c.y0)
		x1, y1 = max(x1, c.x1), max(y1, c.y1)
	}
	if x1 < 0 {
		return 0, 0, 0, 0, 0, false
	}
	gutter := -1
	switch {
	case part[1] < w:
		gutter = part[1]
	case part[0] > 0:
		gutter = part[0]
	}
	return x0, y0, x1, y1, textAngle(cs, gs, why, mh, gutter, p), true
}

// paperTest returns a test for marks that sit on clean paper: the
// non-ink pixels around them are about as light as the page's paper.
// Specks in a binding shadow or on a dark scanner border fail it, so they
// cannot join a page's block the way a page number or a short heading does.
func paperTest(img *image.Gray, thr uint8, p Params) func(comp, int) bool {
	b := img.Bounds()
	var hist [256]int
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for _, v := range img.Pix[y*img.Stride+b.Min.X : y*img.Stride+b.Max.X : y*img.Stride+b.Max.X] {
			hist[v]++
		}
	}
	n := 0
	for v := int(thr); v < 256; v++ {
		n += hist[v]
	}
	paper, acc := 255, 0
	for v := int(thr); v < 256; v++ {
		if acc += hist[v]; 2*acc >= n {
			paper = v
			break
		}
	}
	return func(c comp, mh int) bool {
		m := max(2, mh/2)
		x0, y0 := max(0, c.x0-m), max(0, c.y0-m)
		x1, y1 := min(b.Dx()-1, c.x1+m), min(b.Dy()-1, c.y1+m)
		sum, k := 0, 0
		for y := y0; y <= y1; y++ {
			row := img.Pix[(b.Min.Y+y)*img.Stride+b.Min.X:]
			for x := x0; x <= x1; x++ {
				if v := row[x]; v >= thr {
					sum += int(v)
					k++
				}
			}
		}
		return k > 0 && paper-sum/k <= p.ShadowDelta
	}
}

// flatten returns a copy of the page with each column scaled so its paper
// level (a high percentile of the column, smoothed across neighbouring
// columns) matches the page's: text running into a binding shadow then
// thresholds like the rest. Columns darker than FlatMin, the black borders
// of a scan, and shaded bands narrower than FlatWidth are copied unchanged.
func flatten(img *image.Gray, p Params) *image.Gray {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewGray(image.Rect(0, 0, w, h))
	bg := make([]float64, w)
	var hist [256]int
	for x := 0; x < w; x++ {
		hist = [256]int{}
		n := 0
		for y := 0; y < h; y += 2 {
			hist[img.Pix[(b.Min.Y+y)*img.Stride+b.Min.X+x]]++
			n++
		}
		acc := 0
		for v, c := range hist {
			if acc += c; 10*acc >= 9*n {
				bg[x] = float64(v)
				break
			}
		}
	}
	sm := make([]float64, w)
	const r = 4
	paper := 0.0
	for x := range bg {
		sum, n := 0.0, 0
		for k := max(0, x-r); k <= min(w-1, x+r); k++ {
			sum += bg[k]
			n++
		}
		sm[x] = sum / float64(n)
		paper = math.Max(paper, sm[x])
	}
	// Only wide shadows are flattened: a binding shadow that text runs
	// into spans many columns, while a narrow dark gutter or book-edge band
	// holds no text and flattening it would only turn its texture into ink.
	scale := make([]float64, w)
	for x := range scale {
		scale[x] = 1
	}
	shaded := func(x int) bool { return sm[x] >= float64(p.FlatMin) && sm[x] < 0.92*paper }
	for x := 0; x < w; {
		if !shaded(x) {
			x++
			continue
		}
		e := x
		for e < w && shaded(e) {
			e++
		}
		if float64(e-x) >= p.FlatWidth*float64(w) {
			for k := x; k < e; k++ {
				scale[k] = paper / sm[k]
			}
		}
		x = e
	}
	for y := 0; y < h; y++ {
		src := img.Pix[(b.Min.Y+y)*img.Stride+b.Min.X:]
		dst := out.Pix[y*out.Stride:]
		for x := 0; x < w; x++ {
			dst[x] = uint8(math.Min(255, float64(src[x])*scale[x]))
		}
	}
	return out
}

// skew fits the slope of a text line's baseline (pixels down per pixel to
// the right) from the bottoms of its marks; lines too short to tell get zero
// and fitted false.
func skew(cs []comp, g group, mh int) (float64, bool) {
	if g.n < 6 || g.x1-g.x0 < 10*mh {
		return 0, false
	}
	return fitBaseline(baselinePoints(cs, g, mh, -1, 0), mh)
}

type point struct{ x, y float64 }

// baselinePoints returns the bottom centres of g's glyph-sized marks; with
// gutter >= 0 only those in the part of the line farthest from the gutter
// (share of the line's width), where a book page lies flat.
func baselinePoints(cs []comp, g group, mh, gutter int, share float64) []point {
	lo, hi := g.x0, g.x1
	if gutter >= 0 {
		span := int(share * float64(g.x1-g.x0))
		if gutter >= g.x1 { // gutter to the right: keep the left part
			hi = g.x0 + span
		} else {
			lo = g.x1 - span
		}
	}
	var pts []point
	for _, i := range g.items {
		c := cs[i]
		if 2*(c.y1-c.y0+1) >= mh && c.cx() >= lo && c.cx() <= hi {
			pts = append(pts, point{float64(c.cx()), float64(c.y1)})
		}
	}
	return pts
}

// fitBaseline is a Theil-Sen estimate of the slope through pts, robust to
// descenders and punctuation, refined by least squares on the points that
// sit on the line it found.
func fitBaseline(pts []point, mh int) (float64, bool) {
	if len(pts) < 6 {
		return 0, false
	}
	if len(pts) > 120 { // keep the pair count bounded on very long lines
		step := float64(len(pts)) / 120
		sub := make([]point, 0, 120)
		for k := 0.0; int(k) < len(pts); k += step {
			sub = append(sub, pts[int(k)])
		}
		pts = sub
	}
	var slopes []float64
	for i := range pts {
		for j := i + 1; j < len(pts); j++ {
			if dx := pts[j].x - pts[i].x; math.Abs(dx) >= float64(2*mh) {
				slopes = append(slopes, (pts[j].y-pts[i].y)/dx)
			}
		}
	}
	if len(slopes) < 10 {
		return 0, false
	}
	sort.Float64s(slopes)
	sl := slopes[len(slopes)/2]
	icpt := make([]float64, len(pts))
	for i, q := range pts {
		icpt[i] = q.y - sl*q.x
	}
	sort.Float64s(icpt)
	b0 := icpt[len(icpt)/2]
	var sx, sy, sxx, sxy, n float64
	for _, q := range pts {
		if r := q.y - (b0 + sl*q.x); 3*math.Abs(r) > float64(mh) {
			continue
		}
		sx, sy, sxx, sxy, n = sx+q.x, sy+q.y, sxx+q.x*q.x, sxy+q.x*q.y, n+1
	}
	if den := n*sxx - sx*sx; n >= 6 && den != 0 {
		sl = (n*sxy - sx*sy) / den
	}
	return math.Max(-0.09, math.Min(0.09, sl)), true // at most about 5 degrees
}

// heights returns the sorted heights of components at least 3 pixels tall.
func heights(cs []comp) []int {
	var hs []int
	for _, c := range cs {
		if ch := c.y1 - c.y0 + 1; ch >= 3 {
			hs = append(hs, ch)
		}
	}
	sort.Ints(hs)
	return hs
}

// textAngle is the skew of a block in degrees, as the counterclockwise turn
// that straightens it. It projects the bottom centres of the glyphs of the
// block's core text lines along each candidate angle and keeps the angle at
// which they pile up most sharply into baselines (a projection-profile
// search, which ink bridging two lines cannot fool). On one side of an open
// book only the outer 60% of the block is used, because pages curl towards
// the gutter. Zero when the block holds too little text or the best angle is
// barely better than the others (pictures, warped pages).
func textAngle(cs []comp, gs []group, why []string, mh, gutter int, p Params) float64 {
	x0, x1 := math.MaxInt, -1
	for k, g := range gs {
		if why[k] == "core" {
			x0, x1 = min(x0, g.x0), max(x1, g.x1)
		}
	}
	if x1 < 0 {
		return 0
	}
	lo, hi := x0, x1
	if gutter >= 0 {
		span := int(0.6 * float64(x1-x0))
		if gutter >= x1 {
			hi = x0 + span
		} else {
			lo = x1 - span
		}
	}
	var pts []point
	lines := 0
	for k, g := range gs {
		if why[k] != "core" || g.n < 3 {
			continue
		}
		lines++
		for _, i := range g.items {
			c := cs[i]
			if ch := c.y1 - c.y0 + 1; 2*ch >= mh && ch <= 2*mh && c.cx() >= lo && c.cx() <= hi {
				pts = append(pts, point{float64(c.cx() - lo), float64(c.y1)})
			}
		}
	}
	if lines < 4 || len(pts) < 60 || hi-lo < 15*mh {
		return 0
	}
	score := func(slope float64) float64 {
		bins := map[int]int{}
		for _, q := range pts {
			bins[int(math.Floor(q.y-slope*q.x))]++
		}
		s := 0.0
		for _, n := range bins {
			s += float64(n * n)
		}
		return s
	}
	maxSlope := math.Tan(p.MaxSkew * math.Pi / 180)
	best, bestScore, sum, cnt := 0.0, -1.0, 0.0, 0
	for sl := -maxSlope; sl <= maxSlope+1e-9; sl += 0.0035 { // about 0.2 degrees
		v := score(sl)
		sum, cnt = sum+v, cnt+1
		if v > bestScore {
			best, bestScore = sl, v
		}
	}
	if bestScore < p.SkewContrast*sum/float64(cnt) {
		return 0
	}
	for sl := best - 0.004; sl <= best+0.004; sl += 0.0003 {
		if v := score(sl); v > bestScore {
			best, bestScore = sl, v
		}
	}
	return math.Atan(best) * 180 / math.Pi
}
