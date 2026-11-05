package overlay

import (
	"image"
	"math"
	"sync"
	"testing"
)

func TestTargetSizeMatchesJava(t *testing.T) {
	cases := []struct {
		w, h   float64
		ww, wh int
	}{
		{612, 792, 695, 900},       // Letter: 2550x3300 at 300 DPI
		{960, 540, 1599, 900},      // CREATIVE_COMMONS slides: 3999x2250 at 300 DPI (540*4.1666665f rounds to 2250 in float32), checked in jshell
		{595.28, 841.89, 636, 900}, // A4
		{792, 612, 1164, 900},      // Letter landscape
		{100, 150, 416, 625},       // small page, under the 900 px cap
	}
	for _, c := range cases {
		w, h := TargetSize(c.w, c.h)
		if w != c.ww || h != c.wh {
			t.Errorf("%vx%v pt: got %dx%d want %dx%d", c.w, c.h, w, h, c.ww, c.wh)
		}
	}
}

func gray(w, h int, v uint8) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = v
	}
	return g
}

// reference computes the Java formula directly from the pages.
func reference(pages []*image.Gray) []uint8 {
	n := len(pages)
	out := make([]uint8, len(pages[0].Pix))
	for i := range out {
		sum := 0
		for _, p := range pages {
			sum += int(p.Pix[i])
		}
		mean := sum / n
		dev := 0
		for _, p := range pages {
			d := int(p.Pix[i]) - mean
			dev += d * d
		}
		out[i] = uint8(255 - int(math.Sqrt(float64(dev)/float64(n-1))))
	}
	return out
}

func TestImageMatchesJavaFormula(t *testing.T) {
	const w, h = 40, 30
	var pages []*image.Gray
	for k := 0; k < 7; k++ {
		g := gray(w, h, 255)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if (x+k*3)%11 < 4 || y == 2 {
					g.Pix[y*w+x] = uint8((x*7 + y*13 + k*29) % 256)
				}
			}
		}
		pages = append(pages, g)
	}
	a := NewAccumulator(w, h)
	var wg sync.WaitGroup
	for _, p := range pages {
		wg.Add(1)
		go func(p *image.Gray) { defer wg.Done(); a.Add(p) }(p)
	}
	wg.Wait()
	got := a.Image().Pix
	want := reference(pages)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pixel %d: got %d want %d", i, got[i], want[i])
		}
	}
}

// Identical pages would give a blank sd image; the minimum is used instead.
func TestImageFallsBackToMinimum(t *testing.T) {
	g := gray(20, 20, 255)
	for i := 0; i < 200; i++ {
		g.Pix[i] = 0
	}
	a := NewAccumulator(20, 20)
	a.Add(g)
	a.Add(g)
	a.Add(g)
	out := a.Image()
	if out.Pix[0] != 0 || out.Pix[399] != 255 {
		t.Fatalf("expected the page itself back, got %d and %d", out.Pix[0], out.Pix[399])
	}
}

func TestAddResamplesNearest(t *testing.T) {
	small := gray(10, 10, 255)
	small.Pix[0] = 0
	a := NewAccumulator(20, 20)
	a.Add(small)
	out := a.Image()
	if out.Pix[0] != 0 || out.Pix[1] != 0 || out.Pix[20] != 0 || out.Pix[2] != 255 {
		t.Fatalf("nearest resample wrong: %v", out.Pix[:3])
	}
}

func TestProfiles(t *testing.T) {
	g := gray(4, 2, 255)
	g.Pix[0], g.Pix[4] = 0, 0 // column 0 black
	cols, rows := Profiles(g)
	if cols[0] != 0 || cols[1] != 255 || rows[0] != 191.25 {
		t.Fatalf("cols %v rows %v", cols, rows)
	}
}
