// Package preview draws crop rectangles on an overlay image, numbered in
// output order, for `briss preview` and for reviewing detector results.
package preview

import (
	"image"
	"image/color"

	"github.com/jarofclay/briss-go/internal/geom"
)

var palette = []color.RGBA{{220, 30, 30, 255}, {30, 120, 220, 255}, {20, 160, 60, 255}, {230, 140, 0, 255}, {150, 40, 200, 255}, {0, 160, 170, 255}}

// Draw returns an RGBA copy of g with each rectangle outlined and numbered
// from 1. The area outside the union of the rectangles is tinted so what will
// be cut away is easy to see.
func Draw(g *image.Gray, rects []geom.Ratios) *image.RGBA {
	b := g.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	inside := make([]bool, w*h)
	for _, r := range rects {
		x, y, rw, rh := r.ToPixels(w, h)
		for yy := max(0, int(y)); yy < min(h, int(y+rh)); yy++ {
			for xx := max(0, int(x)); xx < min(w, int(x+rw)); xx++ {
				inside[yy*w+xx] = true
			}
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := g.Pix[(b.Min.Y+y)*g.Stride+b.Min.X+x]
			i := out.PixOffset(x, y)
			if inside[y*w+x] {
				out.Pix[i], out.Pix[i+1], out.Pix[i+2] = v, v, v
			} else { // tint the area that is cut away
				out.Pix[i], out.Pix[i+1], out.Pix[i+2] = uint8(int(v)*3/5+90), uint8(int(v)*3/5+40), uint8(int(v)*3/5+40)
			}
			out.Pix[i+3] = 255
		}
	}
	for k, r := range rects {
		c := palette[k%len(palette)]
		x, y, rw, rh := r.ToPixels(w, h)
		x0, y0, x1, y1 := int(x), int(y), int(x+rw)-1, int(y+rh)-1
		rect(out, x0, y0, x1, y1, c, 3)
		number(out, x0+6, y0+6, k+1, c)
	}
	return out
}

func rect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA, t int) {
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

var digits = [10][7]string{
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

// number draws n (one or two digits) as 5x7 glyphs scaled 3x on a white box.
func number(img *image.RGBA, x, y, n int, c color.RGBA) {
	s := []int{n % 10}
	if n >= 10 {
		s = []int{(n / 10) % 10, n % 10}
	}
	const sc = 3
	for yy := -2; yy < 7*sc+2; yy++ {
		for xx := -2; xx < len(s)*6*sc+2; xx++ {
			img.SetRGBA(x+xx, y+yy, color.RGBA{255, 255, 255, 255})
		}
	}
	for d, digit := range s {
		for r, row := range digits[digit] {
			for col, ch := range row {
				if ch != '1' {
					continue
				}
				for dy := 0; dy < sc; dy++ {
					for dx := 0; dx < sc; dx++ {
						img.SetRGBA(x+d*6*sc+col*sc+dx, y+r*sc+dy, c)
					}
				}
			}
		}
	}
}
