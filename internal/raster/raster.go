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
