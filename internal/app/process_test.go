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

// Two-page sheets and a short chapter-end page: after normalising every
// left page has the same size, every right page too, no rectangle crosses
// into its neighbour, and the short page keeps its top.
func TestNormalize(t *testing.T) {
	pages := []pdf.PageInfo{{Width: 1000, Height: 700}, {Width: 1000, Height: 700}}
	outs := map[int][]crop.Output{
		1: {{Rect: geom.Ratios{L: 0.08, R: 0.55, T: 0.1, B: 0.1}}, {Rect: geom.Ratios{L: 0.56, R: 0.06, T: 0.1, B: 0.12}}},
		2: {{Rect: geom.Ratios{L: 0.1, R: 0.56, T: 0.1, B: 0.5}}, {Rect: geom.Ratios{L: 0.55, R: 0.08, T: 0.11, B: 0.1}}},
	}
	normalize(outs, pages)
	for i := 0; i < 2; i++ {
		w1, h1 := size(outs[1][i].Rect, pages[0])
		w2, h2 := size(outs[2][i].Rect, pages[1])
		if math.Abs(w1-w2) > 1e-6 || math.Abs(h1-h2) > 1e-6 {
			t.Fatalf("rectangle %d: %.1fx%.1f vs %.1fx%.1f", i, w1, h1, w2, h2)
		}
	}
	for p, os := range outs {
		if 1-os[0].Rect.R > os[1].Rect.L+1e-9 {
			t.Fatalf("page %d: left rectangle crosses into the right one: %+v", p, os)
		}
	}
	if math.Abs(outs[2][0].Rect.T-0.1) > 1e-9 {
		t.Fatalf("short page lost its top: %+v", outs[2][0].Rect)
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
// top parts are sized with top parts.
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
	h := func(r geom.Ratios) float64 { return 1 - r.T - r.B }
	if math.Abs(h(outs[1][0].Rect)-h(outs[2][0].Rect)) > 1e-9 {
		t.Fatalf("top parts differ in height: %+v %+v", outs[1][0].Rect, outs[2][0].Rect)
	}
}
