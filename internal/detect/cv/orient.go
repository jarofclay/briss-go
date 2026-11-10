package cv

import (
	"math"
	"sort"
)

// orientation returns the clockwise quarter turn (0, 90, 180 or 270) that
// makes a page's text upright, judged from the ink mask (w x h, true = ink)
// and the kept components, with a confidence between 0 and 1. It answers 0
// when the page holds too little text to tell.
//
// Sideways text is told from upright text by tiles of a few lines each: in
// a tile of horizontal text the ink per row swings between lines and gaps
// while the ink per column stays even, and the other way round for
// vertical text. Upside-down text is told from upright Latin text by its
// glyphs: ascenders (b, d, f, h, k, l, t, capitals, digits) stick out above
// the x-height about four times as often as descenders (g, j, p, q, y) hang
// below the baseline, so in upside-down text the counts swap.
func orientation(mask []bool, w, h int, kept []comp, p Params) (int, float64) {
	if len(kept) < p.OrientMinGlyphs {
		return 0, 0
	}
	x0, y0, x1, y1 := w, h, -1, -1
	var dims []int
	for _, c := range kept {
		x0, y0 = min(x0, c.x0), min(y0, c.y0)
		x1, y1 = max(x1, c.x1), max(y1, c.y1)
		if cw, ch := c.x1-c.x0+1, c.y1-c.y0+1; cw >= 3 || ch >= 3 {
			dims = append(dims, max(cw, ch))
		}
	}
	if len(dims) == 0 {
		return 0, 0
	}
	sort.Ints(dims)
	tile := max(48, min(320, 10*dims[len(dims)/2]))
	horiz, vert := 0, 0
	rows, cols := make([]float64, tile), make([]float64, tile)
	for ty := y0; ty+tile <= y1; ty += tile {
		for tx := x0; tx+tile <= x1; tx += tile {
			for i := range rows {
				rows[i], cols[i] = 0, 0
			}
			ink := 0
			for y := 0; y < tile; y++ {
				row := mask[(ty+y)*w+tx : (ty+y)*w+tx+tile]
				for x, on := range row {
					if on {
						rows[y]++
						cols[x]++
						ink++
					}
				}
			}
			if d := float64(ink) / float64(tile*tile); d < 0.04 || d > 0.5 {
				continue // blank, or a picture
			}
			rcv, ccv := spread(rows), spread(cols)
			switch {
			case rcv > p.OrientTile*ccv:
				horiz++
			case ccv > p.OrientTile*rcv:
				vert++
			}
		}
	}
	vertical := false
	switch {
	case float64(horiz) >= p.OrientVote*float64(vert) && horiz >= 3:
	case float64(vert) >= p.OrientVote*float64(horiz) && vert >= 3:
		vertical = true
	default:
		return 0, 0 // no clear text direction
	}
	cs := kept
	if vertical { // look at the page turned a quarter clockwise
		cs = make([]comp, len(kept))
		for i, c := range kept {
			cs[i] = comp{x0: h - 1 - c.y1, x1: h - 1 - c.y0, y0: c.x0, y1: c.x1, area: c.area}
		}
	}
	asc, desc := ascenders(cs, p)
	if asc+desc < p.OrientMinGlyphs/2 {
		return 0, 0
	}
	score := float64(asc-desc) / float64(asc+desc)
	conf := math.Min(1, math.Abs(score)/0.5)
	switch {
	case score >= p.OrientUpright && vertical:
		return 90, conf
	case score >= p.OrientUpright:
		return 0, conf
	case score <= -p.OrientUpright && vertical:
		return 270, conf
	case score <= -p.OrientUpright:
		return 180, conf
	}
	return 0, 0 // direction clear, but not which way up: leave it
}

// spread is the coefficient of variation of v.
func spread(v []float64) float64 {
	var s, ss float64
	for _, x := range v {
		s += x
		ss += x * x
	}
	n := float64(len(v))
	m := s / n
	if m == 0 {
		return 0
	}
	return math.Sqrt(math.Max(0, ss/n-m*m)) / m
}

// ascenders counts, over the text lines of cs (taken as horizontal), the
// glyphs that rise above the x-height band and those that hang below it.
// The band is found without assuming which way up the text is: its top is
// where most glyph tops sit, its bottom where most glyph bottoms sit, both
// measured along each line's own slope.
func ascenders(cs []comp, p Params) (asc, desc int) {
	gs, mh := lines(cs, p)
	if mh == 0 {
		return 0, 0
	}
	for _, g := range gs {
		if g.n < 8 || g.x1-g.x0 < 10*mh {
			continue
		}
		slope, ok := fitBaseline(baselinePoints(cs, g, mh, -1, 0), mh)
		if !ok {
			slope = 0
		}
		tops := make([]float64, 0, g.n)
		bots := make([]float64, 0, g.n)
		for _, i := range g.items {
			c := cs[i]
			off := slope * float64(c.cx()-g.x0)
			tops = append(tops, float64(c.y0)-off)
			bots = append(bots, float64(c.y1)-off)
		}
		st, sb := append([]float64(nil), tops...), append([]float64(nil), bots...)
		sort.Float64s(st)
		sort.Float64s(sb)
		top, bot := st[len(st)*65/100], sb[len(sb)*35/100]
		xh := bot - top
		if xh < 3 {
			continue
		}
		for k := range tops {
			if tops[k] < top-0.35*xh {
				asc++
			}
			if bots[k] > bot+0.35*xh {
				desc++
			}
		}
	}
	return asc, desc
}

// rowGutter finds where to cut a tall sheet that holds two pages one above
// the other: an empty band of rows near the middle, at least StackGap of
// the height, with each half's content placed the same way in its half (the
// same distance from the half's top and bottom, the same left and right
// edges). A single page with a blank stretch in the middle fails the
// placement test because its text runs on from one half into the other.
func rowGutter(cs []comp, w, h, top, bottom int, p Params) (int, bool) {
	prof := make([]int, h)
	total := 0
	for _, c := range cs {
		per := max(1, c.area/(c.y1-c.y0+1))
		for y := c.y0; y <= c.y1; y++ {
			prof[y] += per
		}
		total += c.area
	}
	peak := 0
	for _, v := range prof {
		peak = max(peak, v)
	}
	if peak == 0 || bottom-top < 20 {
		return 0, false
	}
	limit := max(1, peak/50)
	H := float64(bottom - top)
	ba, bb, start := -1, -1, -1
	for y := top; y <= bottom; y++ {
		empty := y < bottom && prof[y] <= limit
		if empty && start < 0 {
			start = y
		}
		if !empty && start >= 0 {
			mid := float64(start+y) / 2
			if mid >= float64(top)+0.42*H && mid <= float64(top)+0.58*H && float64(y-start) >= p.StackGap*H && y-start > bb-ba {
				ba, bb = start, y
			}
			start = -1
		}
	}
	if ba < 0 {
		return 0, false
	}
	split := (ba + bb) / 2
	type box struct{ x0, y0, x1, y1, n, ink int }
	up := box{x0: w, y0: h, x1: -1, y1: -1}
	dn := up
	for _, c := range cs {
		b := &up
		if c.cy() >= split {
			b = &dn
		}
		b.x0, b.y0 = min(b.x0, c.x0), min(b.y0, c.y0)
		b.x1, b.y1 = max(b.x1, c.x1), max(b.y1, c.y1)
		b.n++
		b.ink += c.area
	}
	for _, b := range []box{up, dn} {
		if b.n < 3*p.MinHalfComps || float64(b.ink) < 0.15*float64(total) {
			return 0, false
		}
	}
	tol, tolX := p.StackAlign*H, p.StackAlign*float64(w)
	abs := func(v int) float64 { return math.Abs(float64(v)) }
	if abs((up.y0-top)-(dn.y0-split)) > tol || abs((split-up.y1)-(bottom-dn.y1)) > tol {
		return 0, false
	}
	if abs(up.x0-dn.x0) > tolX || abs(up.x1-dn.x1) > tolX {
		return 0, false
	}
	return split, true
}
