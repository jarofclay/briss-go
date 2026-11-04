package main

import (
	"image"
	"image/color"
	"sort"
)

// Ratios are briss crop ratios (fractions from left, bottom, right, top edges).
type Ratios struct {
	L, B, R, T float64
}

// Candidate is one proposed crop rectangle with a label explaining how it was built.
type Candidate struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
	Rect  Ratios `json:"rect"`
}

// inkProfile returns per-row and per-column counts of "ink" pixels (value < thr).
func inkProfile(img *image.Gray, thr uint8) (rows, cols []int) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	rows = make([]int, h)
	cols = make([]int, w)
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+w]
		for x, p := range row {
			if p < thr {
				rows[y]++
				cols[x]++
			}
		}
	}
	return
}

// bands finds runs of consecutive indices whose count >= minCount, separated by
// gaps of at least minGap empty indices. Returns [start,end) pairs.
func bands(profile []int, minCount, minGap int) [][2]int {
	var out [][2]int
	start, gap := -1, 0
	for i, v := range profile {
		if v >= minCount {
			if start < 0 {
				start = i
			}
			gap = 0
		} else if start >= 0 {
			gap++
			if gap >= minGap {
				out = append(out, [2]int{start, i - gap + 1})
				start, gap = -1, 0
			}
		}
	}
	if start >= 0 {
		out = append(out, [2]int{start, len(profile) - gap})
	}
	return out
}

// buildCandidates derives candidate rectangles from the overlay:
//  0 legacy auto-crop (Java algorithm)
//  1 ink bounding box (everything dark)
//  2 ink bbox without the top band (header stripped)
//  3 ink bbox without the bottom band (footer stripped)
//  4 ink bbox without top and bottom bands
//  5 (4) without narrow side columns (margin notes stripped)
// Variants are only emitted when they differ from a previous candidate.
func buildCandidates(img *image.Gray) []Candidate {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	rows, cols := inkProfile(img, 200)
	var out []Candidate
	add := func(label string, r Ratios) {
		for _, c := range out {
			if approxEq(c.Rect, r, 0.004) {
				return
			}
		}
		out = append(out, Candidate{ID: len(out), Label: label, Rect: r})
	}
	add("legacy auto-crop", legacyAutoCrop(img))

	minRow := max(2, w/200)
	minCol := max(2, h/200)
	rb := bands(rows, minRow, max(4, h/120))
	cb := bands(cols, minCol, max(4, w/120))
	if len(rb) == 0 || len(cb) == 0 {
		return out
	}
	toRatios := func(x0, y0, x1, y1 int) Ratios {
		pad := 2.0
		return Ratios{L: clamp((float64(x0) - pad) / float64(w)), T: clamp((float64(y0) - pad) / float64(h)),
			R: clamp((float64(w-x1) - pad) / float64(w)), B: clamp((float64(h-y1) - pad) / float64(h))}
	}
	x0, x1 := cb[0][0], cb[len(cb)-1][1]
	y0, y1 := rb[0][0], rb[len(rb)-1][1]
	add("ink bounding box", toRatios(x0, y0, x1, y1))
	// header/footer bands: thin first/last row bands separated from the main block
	hdrEnd, ftrStart := y0, y1
	if len(rb) >= 2 && rb[0][1]-rb[0][0] < h/25 {
		hdrEnd = rb[1][0]
	}
	if len(rb) >= 2 && rb[len(rb)-1][1]-rb[len(rb)-1][0] < h/25 {
		ftrStart = rb[len(rb)-2][1]
	}
	add("without top band", toRatios(x0, hdrEnd, x1, y1))
	add("without bottom band", toRatios(x0, y0, x1, ftrStart))
	add("without top and bottom bands", toRatios(x0, hdrEnd, x1, ftrStart))
	// side columns: recompute column bands within the body rows only
	_, cols2 := inkProfile(crop(img, 0, hdrEnd, w, ftrStart), 200)
	cb2 := bands(cols2, max(2, (ftrStart-hdrEnd)/200), max(4, w/120))
	if len(cb2) >= 2 {
		// drop narrow outer columns (less than 20% of the widest)
		widest := 0
		for _, b := range cb2 {
			widest = max(widest, b[1]-b[0])
		}
		var keep [][2]int
		for _, b := range cb2 {
			if b[1]-b[0] >= widest/5 {
				keep = append(keep, b)
			}
		}
		if len(keep) > 0 && (keep[0][0] != cb2[0][0] || keep[len(keep)-1][1] != cb2[len(cb2)-1][1]) {
			add("without narrow side columns", toRatios(keep[0][0], hdrEnd, keep[len(keep)-1][1], ftrStart))
		}
	}
	return out
}

func crop(img *image.Gray, x0, y0, x1, y1 int) *image.Gray {
	return img.SubImage(image.Rect(x0, y0, x1, y1)).(*image.Gray)
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 0.95 {
		return 0.95
	}
	return v
}

func approxEq(a, b Ratios, eps float64) bool {
	d := func(x, y float64) bool { return x-y < eps && y-x < eps }
	return d(a.L, b.L) && d(a.B, b.B) && d(a.R, b.R) && d(a.T, b.T)
}

func iou(a, b Ratios) float64 {
	ax0, ay0, ax1, ay1 := a.L, a.T, 1-a.R, 1-a.B
	bx0, by0, bx1, by1 := b.L, b.T, 1-b.R, 1-b.B
	iw := min(ax1, bx1) - max(ax0, bx0)
	ih := min(ay1, by1) - max(ay0, by0)
	if iw <= 0 || ih <= 0 {
		return 0
	}
	inter := iw * ih
	return inter / ((ax1-ax0)*(ay1-ay0) + (bx1-bx0)*(by1-by0) - inter)
}

// clipped reports whether pred cuts into gt by more than tol (fraction of page).
func clipped(pred, gt Ratios, tol float64) bool {
	return pred.L > gt.L+tol || pred.B > gt.B+tol || pred.R > gt.R+tol || pred.T > gt.T+tol
}

// drawCandidates paints numbered rectangles on an RGBA copy of the gray image.
func drawCandidates(g *image.Gray, cands []Candidate) *image.RGBA {
	b := g.Bounds()
	out := image.NewRGBA(b)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			v := g.Pix[y*g.Stride+x]
			i := out.PixOffset(x, y)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = v, v, v, 255
		}
	}
	palette := []color.RGBA{{220, 30, 30, 255}, {30, 120, 220, 255}, {20, 160, 60, 255}, {230, 140, 0, 255}, {150, 40, 200, 255}, {0, 160, 170, 255}, {120, 80, 0, 255}}
	w, h := b.Dx(), b.Dy()
	for _, c := range cands {
		col := palette[c.ID%len(palette)]
		x0, y0 := int(c.Rect.L*float64(w)), int(c.Rect.T*float64(h))
		x1, y1 := int((1-c.Rect.R)*float64(w)), int((1-c.Rect.B)*float64(h))
		inset := c.ID * 3 // nested outlines stay distinguishable
		drawRect(out, x0+inset, y0+inset, x1-inset, y1-inset, col, 2)
		drawDigit(out, x0+inset+6, y0+inset+6, c.ID, col)
	}
	return out
}

func drawRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA, t int) {
	for k := 0; k < t; k++ {
		for x := x0; x <= x1; x++ {
			img.SetRGBA(x, y0+k, c)
			img.SetRGBA(x, y1-k, c)
		}
		for y := y0; y <= y1; y++ {
			img.SetRGBA(x0+k, y, c)
			img.SetRGBA(x1-k, y, c)
		}
	}
}

// drawDigit renders a 0-9 label as a 5x7 bitmap scaled 4x inside a white box.
func drawDigit(img *image.RGBA, x, y, n int, c color.RGBA) {
	font := [10][7]string{
		{"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
		{"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
		{"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
		{"11111", "00010", "00100", "00010", "00001", "10001", "01110"},
		{"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
		{"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
		{"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
		{"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
		{"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
		{"01110", "10001", "10001", "01111", "00001", "00010", "01100"},
	}
	s := 4
	for yy := -3; yy < 7*s+3; yy++ {
		for xx := -3; xx < 5*s+3; xx++ {
			img.SetRGBA(x+xx, y+yy, color.RGBA{255, 255, 255, 255})
		}
	}
	for r, row := range font[n%10] {
		for col, ch := range row {
			if ch == '1' {
				for dy := 0; dy < s; dy++ {
					for dx := 0; dx < s; dx++ {
						img.SetRGBA(x+col*s+dx, y+r*s+dy, c)
					}
				}
			}
		}
	}
}

func sortCandidates(c []Candidate) { sort.Slice(c, func(i, j int) bool { return c[i].ID < c[j].ID }) }
