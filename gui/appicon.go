package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"fyne.io/fyne/v2"
)

// appIcon draws the app icon: crop marks on a black rounded square.
func appIcon() fyne.Resource {
	const n = 256
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	bg, fg, ac := colBackground, colText, colAccent
	rad := 56.0
	inside := func(x, y float64) bool {
		cx := math.Max(rad, math.Min(n-rad, x))
		cy := math.Max(rad, math.Min(n-rad, y))
		return math.Hypot(x-cx, y-cy) <= rad
	}
	set := func(x, y int, c color.NRGBA) {
		if x >= 0 && y >= 0 && x < n && y < n {
			img.SetNRGBA(x, y, c)
		}
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if inside(float64(x)+0.5, float64(y)+0.5) {
				set(x, y, bg)
			}
		}
	}
	bar := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				set(x, y, c)
			}
		}
	}
	// two corner brackets, like crop marks
	t, l := 14, 62
	bar(56, 56, 56+l, 56+t, fg)
	bar(56, 56, 56+t, 56+l, fg)
	bar(n-56-l, n-56-t, n-56, n-56, fg)
	bar(n-56-t, n-56-l, n-56, n-56, fg)
	// the page, a soft block between them
	bar(96, 90, n-96, 102, withAlpha(fg, 0x50))
	bar(96, 114, n-96, 126, withAlpha(fg, 0x50))
	bar(96, 138, n-120, 150, withAlpha(fg, 0x50))
	for y := -10; y <= 10; y++ { // accent dot
		for x := -10; x <= 10; x++ {
			if x*x+y*y <= 100 {
				set(n-80+x, 80+y, ac)
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return fyne.NewStaticResource("briss.png", buf.Bytes())
}
