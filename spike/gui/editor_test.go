package main

import (
	"image/png"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestEditorHeadless drives the editor with synthetic input on the software
// test driver, captures a screenshot and times refresh+capture cycles as a
// proxy for frame cost without a GPU.
func TestEditorHeadless(t *testing.T) {
	a := test.NewTempApp(t)
	_ = a
	img := loadPNG("../testdata/overlays/book300_c00_odd_612x792_p1.png")
	ed := NewCropEditor(img)
	w := test.NewTempWindow(t, ed)
	w.Resize(ed.MinSize())
	c := w.Canvas()

	// drag simulates the GLFW driver: each Dragged event carries the current
	// pointer position and the delta since the previous event; the widget
	// derives the press position from the first event.
	drag := func(from fyne.Position, dx, dy float32) {
		half := fyne.NewPos(from.X+dx/2, from.Y+dy/2)
		ed.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: half}, Dragged: fyne.Delta{DX: dx / 2, DY: dy / 2}})
		ed.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(from.X+dx, from.Y+dy)}, Dragged: fyne.Delta{DX: dx / 2, DY: dy / 2}})
		ed.DragEnd()
	}
	// draw: drag from (90,110) to (620,800)
	drag(fyne.NewPos(90, 110), 530, 690)
	if len(ed.Rects) != 1 {
		t.Fatalf("expected 1 rect after draw, got %d", len(ed.Rects))
	}
	r := *ed.Rects[0]
	// move by (10, 15) dragging inside
	drag(fyne.NewPos(300, 400), 10, 15)
	if ed.Rects[0].X != r.X+10 || ed.Rects[0].Y != r.Y+15 {
		t.Fatalf("move failed: %+v -> %+v", r, *ed.Rects[0])
	}
	// resize via SE handle
	se := handlePositions(ed.Rects[0])[4]
	drag(se, -20, -30)
	if ed.Rects[0].W != r.W-20 || ed.Rects[0].H != r.H-30 {
		t.Fatalf("resize failed: %+v", *ed.Rects[0])
	}
	// draw a second small rect, select it with a tap, nudge with keys
	// second rectangle in the free area right of the first one
	drag(fyne.NewPos(560, 30), 100, 80)
	if len(ed.Rects) != 2 {
		t.Fatalf("expected 2 rects, got %d", len(ed.Rects))
	}
	ed.Tapped(&fyne.PointEvent{Position: fyne.NewPos(600, 60)})
	if !ed.Rects[1].Selected {
		t.Fatalf("tap should select the second rectangle")
	}
	ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})

	// hover over a corner handle -> diagonal cursor
	mev := desktopMouseEvent(handlePositions(ed.Rects[0])[0])
	ed.MouseMoved(&mev)
	if _, ok := ed.Cursor().(*bitmapCursor); !ok {
		t.Fatalf("expected custom diagonal cursor on NW handle, got %T", ed.Cursor())
	}

	// screenshot
	shot := c.Capture()
	os.MkdirAll("../testdata/gui", 0o755)
	f, err := os.Create("../testdata/gui/editor_headless.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, shot); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// frame cost proxy: refresh + software capture, 30 cycles
	const n = 30
	t0 := time.Now()
	for i := 0; i < n; i++ {
		ed.Rects[0].X += 1
		ed.Refresh()
		_ = c.Capture()
	}
	per := time.Since(t0) / n
	t.Logf("refresh+software-capture per frame: %v (%d objects)", per, len(ed.Rects)*11+1)
	if err := os.WriteFile("../testdata/gui/frame_cost.txt", []byte(per.String()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
