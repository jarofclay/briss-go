// Spike 4: Fyne crop-editor prototype. Run on a desktop:
//
//	go run . ../testdata/overlays/book300_c00_odd_612x792_p1.png
//
// Drag on empty space to draw, drag inside to move, drag a handle to resize,
// right-click to delete, arrows nudge, +/- resize, Escape deselects.
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func loadPNG(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		panic(err)
	}
	return img
}

func main() {
	path := "../testdata/overlays/book300_c00_odd_612x792_p1.png"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	a := app.NewWithID("briss-go-spike-gui")
	w := a.NewWindow("briss-go crop editor spike")
	ed := NewCropEditor(loadPNG(path))
	status := widget.NewLabel("draw a rectangle")
	ed.OnChange = func() {
		s := ""
		for i, r := range ed.Rects {
			s += fmt.Sprintf("[%d] x=%.0f y=%.0f w=%.0f h=%.0f  ", i, r.X, r.Y, r.W, r.H)
		}
		status.SetText(s)
	}
	w.SetContent(container.NewBorder(nil, status, nil, nil, container.NewScroll(ed)))
	w.Canvas().Focus(ed)
	w.Resize(fyne.NewSize(900, 1000))
	w.ShowAndRun()
}
