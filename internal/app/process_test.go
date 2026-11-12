package app

import (
	"math"
	"testing"

	"github.com/jarofclay/briss-go/internal/crop"
	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/jarofclay/briss-go/internal/pdf"
)

func size(r geom.Ratios, pi pdf.PageInfo) (float64, float64) {
	return (1 - r.L - r.R) * pi.Width, (1 - r.T - r.B) * pi.Height
}

func TestStraightRectInvertsTheTilt(t *testing.T) {
	const pw, ph = 600.0, 800.0
	w, h, deg := 300.0, 500.0, 3.0
	s, c := math.Sincos(deg * math.Pi / 180)
	W, H := w*c+h*s, w*s+h*c
	r := geom.Ratios{L: (pw/2 - W/2) / pw, R: (pw/2 - W/2) / pw, T: (ph/2 - H/2) / ph, B: (ph/2 - H/2) / ph}
	got, ok := straightRect(r, -deg, pw, ph)
	gw, gh := size(got, pdf.PageInfo{Width: pw, Height: ph})
	if !ok || math.Abs(gw-w) > 1e-6 || math.Abs(gh-h) > 1e-6 {
		t.Fatalf("got %.3f x %.3f, want %v x %v", gw, gh, w, h)
	}
}

// outSize is the size of the page an output becomes: its Box when it has one.
func outSize(o crop.Output, pi pdf.PageInfo) (float64, float64) {
	if o.Box != nil {
		return size(*o.Box, pi)
	}
	return size(o.Rect, pi)
}

// Two-page sheets and a short chapter-end page: after normalising every page,
// left or right, has the same size, no rectangle crosses into its neighbour,
// and the short page keeps its top.
func TestNormalize(t *testing.T) {
	pages := []pdf.PageInfo{{Width: 1000, Height: 700}, {Width: 1000, Height: 700}}
	outs := map[int][]crop.Output{
		1: {{Rect: geom.Ratios{L: 0.08, R: 0.55, T: 0.1, B: 0.1}}, {Rect: geom.Ratios{L: 0.56, R: 0.06, T: 0.1, B: 0.12}}},
		2: {{Rect: geom.Ratios{L: 0.1, R: 0.56, T: 0.1, B: 0.5}}, {Rect: geom.Ratios{L: 0.55, R: 0.08, T: 0.11, B: 0.1}}},
	}
	normalize(outs, pages)
	w0, h0 := outSize(outs[1][0], pages[0])
	for p, os := range outs {
		for i, o := range os {
			if w, h := outSize(o, pages[p-1]); math.Abs(w-w0) > 1e-6 || math.Abs(h-h0) > 1e-6 {
				t.Fatalf("page %d rectangle %d: %.1fx%.1f, want %.1fx%.1f", p, i, w, h, w0, h0)
			}
		}
		if 1-os[0].Rect.R > os[1].Rect.L+1e-9 {
			t.Fatalf("page %d: left rectangle crosses into the right one: %+v", p, os)
		}
	}
	if math.Abs(w0-380) > 1e-6 || math.Abs(h0-560) > 1e-6 {
		t.Fatalf("size %.1fx%.1f, want the largest, 380x560", w0, h0)
	}
	if math.Abs(outs[2][0].Rect.T-0.1) > 1e-9 {
		t.Fatalf("short page lost its top: %+v", outs[2][0].Rect)
	}
}

// Where a page has no room to grow (a spread whose pages fill their halves,
// next to a wider single page), the output page gets a Box of the common size
// while what it shows stays on its own half. A landscape page (a fold-out map)
// is sized on its own and does not widen the others.
func TestNormalizePadsWhereThePageEnds(t *testing.T) {
	pages := []pdf.PageInfo{{Width: 1000, Height: 700}, {Width: 700, Height: 700}, {Width: 1000, Height: 600}}
	outs := map[int][]crop.Output{
		1: {{Rect: geom.Ratios{L: 0.02, R: 0.52, T: 0.1, B: 0.1}}, {Rect: geom.Ratios{L: 0.52, R: 0.02, T: 0.1, B: 0.1}}},
		2: {{Rect: geom.Ratios{L: 50.0 / 700, R: 50.0 / 700, T: 0.1, B: 0.1}}},
		3: {{Rect: geom.Ratios{L: 0.05, R: 0.05, T: 0.05, B: 0.05}}},
	}
	normalize(outs, pages)
	for p := 1; p <= 2; p++ {
		for i, o := range outs[p] {
			if w, h := outSize(o, pages[p-1]); math.Abs(w-600) > 1e-6 || math.Abs(h-560) > 1e-6 {
				t.Fatalf("page %d rectangle %d: %.1fx%.1f, want 600x560", p, i, w, h)
			}
		}
	}
	if w, h := outSize(outs[3][0], pages[2]); math.Abs(w-900) > 1e-6 || math.Abs(h-540) > 1e-6 || outs[3][0].Box != nil {
		t.Fatalf("the landscape page changed: %.1fx%.1f %+v", w, h, outs[3][0])
	}
	left, right := outs[1][0], outs[1][1]
	if left.Box == nil || right.Box == nil {
		t.Fatalf("the spread's pages need a larger page box: %+v", outs[1])
	}
	if 1-left.Rect.R > 0.5+1e-9 || right.Rect.L < 0.5-1e-9 {
		t.Fatalf("what shows crosses the gutter: left %+v right %+v", left.Rect, right.Rect)
	}
	for _, o := range outs[1] {
		b, r := *o.Box, o.Rect
		if b.L > r.L || b.R > r.R || b.T > r.T || b.B > r.B {
			t.Fatalf("box %+v does not hold %+v", b, r)
		}
	}
	if outs[2][0].Box != nil {
		t.Fatalf("the widest page needs no box: %+v", outs[2][0])
	}
}

func TestOutputAtFollowsTheTurn(t *testing.T) {
	pi := pdf.PageInfo{Width: 100, Height: 100}
	os := []crop.Output{{Rect: geom.Ratios{L: 0.4, R: 0.4, T: 0, B: 0}, Angle: 90}}
	// a quarter turn counterclockwise takes a point right of the centre to
	// above it (inside the tall strip) and a point above it to its left (out)
	if outputAt(os, 70, 50, pi) != 0 || outputAt(os, 50, 20, pi) != -1 {
		t.Fatalf("turned membership wrong")
	}
	flat := []crop.Output{{Rect: os[0].Rect}}
	if outputAt(flat, 70, 50, pi) != -1 || outputAt(flat, 50, 20, pi) != 0 {
		t.Fatal("unturned membership wrong")
	}
}

// Pages split into a top and a bottom part keep apart when normalised, and
// every part gets the same height.
func TestNormalizeRows(t *testing.T) {
	pages := []pdf.PageInfo{{Width: 600, Height: 900}, {Width: 600, Height: 900}}
	outs := map[int][]crop.Output{
		1: {{Rect: geom.Ratios{L: 0.1, R: 0.1, T: 0.08, B: 0.52}}, {Rect: geom.Ratios{L: 0.1, R: 0.1, T: 0.5, B: 0.1}}},
		2: {{Rect: geom.Ratios{L: 0.12, R: 0.1, T: 0.08, B: 0.55}}, {Rect: geom.Ratios{L: 0.1, R: 0.12, T: 0.47, B: 0.3}}},
	}
	normalize(outs, pages)
	for p, os := range outs {
		if 1-os[0].Rect.B > os[1].Rect.T+1e-9 {
			t.Fatalf("page %d: top part runs into the bottom part: %+v", p, os)
		}
	}
	_, h0 := outSize(outs[1][0], pages[0])
	for p, os := range outs {
		for i, o := range os {
			if _, h := outSize(o, pages[p-1]); math.Abs(h-h0) > 1e-6 {
				t.Fatalf("page %d part %d is %.1f high, want %.1f", p, i, h, h0)
			}
		}
	}
}
