// Package raster holds small operations on page images shared by the
// detectors, the OCR step and the GUI preview.
package raster

import (
	"image"
	"math"
)

// Rotate returns src turned by deg degrees counterclockwise (as seen on
// screen) about the point (cx, cy) in src pixels, same size as src; pixels
// that come from outside src are bg. Bilinear sampling.
func Rotate(src *image.Gray, deg, cx, cy float64, bg uint8) *image.Gray {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewGray(image.Rect(0, 0, w, h))
	sin, cos := math.Sincos(deg * math.Pi / 180)
	for y := 0; y < h; y++ {
		row := dst.Pix[y*dst.Stride:]
		dy := float64(y) + 0.5 - cy
		for x := 0; x < w; x++ {
			dx := float64(x) + 0.5 - cx
			// inverse of the on-screen counterclockwise turn (y points down)
			sx := cx + dx*cos - dy*sin - 0.5
			sy := cy + dx*sin + dy*cos - 0.5
			row[x] = sample(src, sx, sy, bg)
		}
	}
	return dst
}

func sample(g *image.Gray, x, y float64, bg uint8) uint8 {
	b := g.Bounds()
	w, h := b.Dx(), b.Dy()
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	at := func(xx, yy int) float64 {
		if xx < 0 || yy < 0 || xx >= w || yy >= h {
			return float64(bg)
		}
		return float64(g.Pix[(b.Min.Y+yy)*g.Stride+b.Min.X+xx])
	}
	v := at(x0, y0)*(1-fx)*(1-fy) + at(x0+1, y0)*fx*(1-fy) + at(x0, y0+1)*(1-fx)*fy + at(x0+1, y0+1)*fx*fy
	return uint8(math.Round(math.Max(0, math.Min(255, v))))
}

// Crop returns a copy of the pixels of r (clipped to src).
func Crop(src *image.Gray, r image.Rectangle) *image.Gray {
	r = r.Intersect(src.Bounds())
	dst := image.NewGray(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+r.Dx()], src.Pix[(r.Min.Y+y)*src.Stride+r.Min.X:])
	}
	return dst
}

// RotateQuarter returns src turned clockwise by turn degrees (0, 90, 180 or
// 270), exactly, pixel for pixel.
func RotateQuarter(src *image.Gray, turn int) *image.Gray {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	at := func(x, y int) uint8 { return src.Pix[(b.Min.Y+y)*src.Stride+b.Min.X+x] }
	switch ((turn % 360) + 360) % 360 {
	case 90:
		dst := image.NewGray(image.Rect(0, 0, h, w))
		for y := 0; y < w; y++ {
			for x := 0; x < h; x++ {
				dst.Pix[y*dst.Stride+x] = at(y, h-1-x)
			}
		}
		return dst
	case 180:
		dst := image.NewGray(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Pix[y*dst.Stride+x] = at(w-1-x, h-1-y)
			}
		}
		return dst
	case 270:
		dst := image.NewGray(image.Rect(0, 0, h, w))
		for y := 0; y < w; y++ {
			for x := 0; x < h; x++ {
				dst.Pix[y*dst.Stride+x] = at(w-1-y, x)
			}
		}
		return dst
	}
	return Crop(src, b)
}

// Resize returns src scaled to w x h pixels by averaging the source pixels
// each destination pixel covers (meant for shrinking; enlarging repeats
// pixels).
func Resize(src *image.Gray, w, h int) *image.Gray {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || (w == sw && h == sh) {
		return Crop(src, b)
	}
	// horizontal pass into a float buffer, then vertical
	tmp := make([]float64, w*sh)
	fx := float64(sw) / float64(w)
	for y := 0; y < sh; y++ {
		row := src.Pix[(b.Min.Y+y)*src.Stride+b.Min.X:]
		for x := 0; x < w; x++ {
			tmp[y*w+x] = area(func(i int) float64 { return float64(row[i]) }, float64(x)*fx, float64(x+1)*fx, sw)
		}
	}
	dst := image.NewGray(image.Rect(0, 0, w, h))
	fy := float64(sh) / float64(h)
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			v := area(func(i int) float64 { return tmp[i*w+x] }, float64(y)*fy, float64(y+1)*fy, sh)
			dst.Pix[y*dst.Stride+x] = uint8(math.Round(math.Max(0, math.Min(255, v))))
		}
	}
	return dst
}

// area averages at(i) over the interval [a, b) of a line of n samples,
// weighting the samples at either end by how much of them it covers.
func area(at func(int) float64, a, b float64, n int) float64 {
	if b-a < 1e-9 {
		return at(min(n-1, int(a)))
	}
	sum, wt := 0.0, 0.0
	for i := int(math.Floor(a)); float64(i) < b && i < n; i++ {
		lo, hi := math.Max(a, float64(i)), math.Min(b, float64(i+1))
		if hi > lo {
			sum += at(i) * (hi - lo)
			wt += hi - lo
		}
	}
	if wt == 0 {
		return at(min(n-1, int(a)))
	}
	return sum / wt
}
