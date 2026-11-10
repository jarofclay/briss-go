package main

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2/driver/desktop"
)

// Fyne ships horizontal and vertical resize cursors but no diagonal ones, so the
// corner handles use custom cursor images (desktop.Cursor is an interface).
type bitmapCursor struct {
	img  *image.NRGBA
	x, y int
}

func (c *bitmapCursor) Image() (image.Image, int, int) { return c.img, c.x, c.y }

// diagonalCursor draws a 24x24 double-headed arrow along the main (nwse=true) or
// anti (nwse=false) diagonal with a white outline for visibility on any background.
func diagonalCursor(nwse bool) desktop.Cursor {
	const n = 24
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	set := func(x, y int, c color.NRGBA) {
		if x >= 0 && y >= 0 && x < n && y < n {
			img.SetNRGBA(x, y, c)
		}
	}
	black, white := color.NRGBA{0, 0, 0, 255}, color.NRGBA{255, 255, 255, 255}
	px := func(x, y int) {
		if !nwse {
			x = n - 1 - x
		}
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				set(x+dx, y+dy, white)
			}
		}
	}
	for i := 3; i < n-3; i++ {
		px(i, i)
	}
	for i := 0; i < 7; i++ { // arrow heads
		px(3+i, 3)
		px(3, 3+i)
		px(n-4-i, n-4)
		px(n-4, n-4-i)
	}
	// black core on top of the white outline
	core := func(x, y int) {
		if !nwse {
			x = n - 1 - x
		}
		set(x, y, black)
	}
	for i := 3; i < n-3; i++ {
		core(i, i)
	}
	for i := 0; i < 7; i++ {
		core(3+i, 3)
		core(3, 3+i)
		core(n-4-i, n-4)
		core(n-4, n-4-i)
	}
	return &bitmapCursor{img: img, x: n / 2, y: n / 2}
}

var handleCursors = [8]desktop.Cursor{
	diagonalCursor(true), desktop.VResizeCursor, diagonalCursor(false), desktop.HResizeCursor,
	diagonalCursor(true), desktop.VResizeCursor, diagonalCursor(false), desktop.HResizeCursor,
}
