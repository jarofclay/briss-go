package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"sort"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// CropRect is a rectangle in image pixel coordinates (top-left origin).
type CropRect struct {
	X, Y, W, H float32
	Selected   bool
}

type dragMode int

const (
	modeNone dragMode = iota
	modeDraw
	modeMove
	modeResize
)

// handle ids: 0..7 = NW, N, NE, E, SE, S, SW, W; -1 = none
const handleSize = float32(8)

// CropEditor shows an image with editable crop rectangles: draw by dragging on
// empty space, move by dragging inside a rectangle, resize by dragging one of the
// eight handles, select by tap, delete by right-click or the Delete key, nudge with
// the arrow keys, Escape deselects. Custom cursors for the eight handles.
type CropEditor struct {
	widget.BaseWidget
	img        image.Image
	Rects      []*CropRect
	ghost      *CropRect // size of the last drawn rectangle
	mode       dragMode
	active     int
	handle     int
	start      fyne.Position
	orig       CropRect
	cursor     desktop.Cursor
	OnChange   func()
	frameCount int
}

func NewCropEditor(img image.Image) *CropEditor {
	e := &CropEditor{img: img, active: -1, handle: -1, cursor: desktop.CrosshairCursor}
	e.ExtendBaseWidget(e)
	return e
}

func (e *CropEditor) imageSize() fyne.Size {
	b := e.img.Bounds()
	return fyne.NewSize(float32(b.Dx()), float32(b.Dy()))
}

func (e *CropEditor) MinSize() fyne.Size { return e.imageSize() }

// hitTest returns the rectangle index and handle under p (handle -1 = inside body).
func (e *CropEditor) hitTest(p fyne.Position) (int, int) {
	for i := len(e.Rects) - 1; i >= 0; i-- { // newest first, like Briss 2.0
		r := e.Rects[i]
		for h, hp := range handlePositions(r) {
			if abs(p.X-hp.X) <= handleSize && abs(p.Y-hp.Y) <= handleSize {
				return i, h
			}
		}
		if p.X >= r.X && p.X <= r.X+r.W && p.Y >= r.Y && p.Y <= r.Y+r.H {
			return i, -1
		}
	}
	return -1, -1
}

func handlePositions(r *CropRect) [8]fyne.Position {
	return [8]fyne.Position{
		{X: r.X, Y: r.Y}, {X: r.X + r.W/2, Y: r.Y}, {X: r.X + r.W, Y: r.Y}, {X: r.X + r.W, Y: r.Y + r.H/2},
		{X: r.X + r.W, Y: r.Y + r.H}, {X: r.X + r.W/2, Y: r.Y + r.H}, {X: r.X, Y: r.Y + r.H}, {X: r.X, Y: r.Y + r.H/2},
	}
}

// --- fyne.Draggable
func (e *CropEditor) Dragged(ev *fyne.DragEvent) {
	if e.mode == modeNone {
		e.start = ev.Position.Subtract(fyne.NewPos(ev.Dragged.DX, ev.Dragged.DY))
		i, h := e.hitTest(e.start)
		switch {
		case i >= 0 && h >= 0:
			e.mode, e.active, e.handle, e.orig = modeResize, i, h, *e.Rects[i]
		case i >= 0:
			e.mode, e.active, e.orig = modeMove, i, *e.Rects[i]
		default:
			r := &CropRect{X: e.start.X, Y: e.start.Y, Selected: true}
			e.deselectAll()
			e.Rects = append(e.Rects, r)
			e.mode, e.active = modeDraw, len(e.Rects)-1
		}
	}
	r := e.Rects[e.active]
	dx, dy := ev.Position.X-e.start.X, ev.Position.Y-e.start.Y
	switch e.mode {
	case modeDraw:
		r.X, r.Y, r.W, r.H = minf(e.start.X, ev.Position.X), minf(e.start.Y, ev.Position.Y), abs(dx), abs(dy)
	case modeMove:
		r.X, r.Y = e.orig.X+dx, e.orig.Y+dy
	case modeResize:
		o := e.orig
		switch e.handle {
		case 0:
			r.X, r.Y, r.W, r.H = o.X+dx, o.Y+dy, o.W-dx, o.H-dy
		case 1:
			r.Y, r.H = o.Y+dy, o.H-dy
		case 2:
			r.Y, r.W, r.H = o.Y+dy, o.W+dx, o.H-dy
		case 3:
			r.W = o.W + dx
		case 4:
			r.W, r.H = o.W+dx, o.H+dy
		case 5:
			r.H = o.H + dy
		case 6:
			r.X, r.W, r.H = o.X+dx, o.W-dx, o.H+dy
		case 7:
			r.X, r.W = o.X+dx, o.W-dx
		}
	}
	e.clamp(r)
	e.Refresh()
}

func (e *CropEditor) DragEnd() {
	if e.active >= 0 && e.active < len(e.Rects) {
		r := e.Rects[e.active]
		if r.W < 5 || r.H < 5 { // too small: drop (Briss colours it red while dragging)
			e.Rects = append(e.Rects[:e.active], e.Rects[e.active+1:]...)
		} else if e.mode == modeDraw {
			g := *r
			e.ghost = &g
		}
	}
	e.mode, e.active, e.handle = modeNone, -1, -1
	e.changed()
	e.Refresh()
}

func (e *CropEditor) clamp(r *CropRect) {
	s := e.imageSize()
	if r.W < 0 {
		r.X, r.W = r.X+r.W, -r.W
	}
	if r.H < 0 {
		r.Y, r.H = r.Y+r.H, -r.H
	}
	r.X, r.Y = maxf(0, r.X), maxf(0, r.Y)
	if r.X+r.W > s.Width {
		r.W = s.Width - r.X
	}
	if r.Y+r.H > s.Height {
		r.H = s.Height - r.Y
	}
}

// --- fyne.Tappable / SecondaryTappable
func (e *CropEditor) Tapped(ev *fyne.PointEvent) {
	// single click selects exactly one rectangle (Briss 2.0 semantics);
	// multi-select with shift-click is left for the real implementation.
	i, _ := e.hitTest(ev.Position)
	e.deselectAll()
	if i >= 0 {
		e.Rects[i].Selected = true
	}
	e.Refresh()
}

func (e *CropEditor) TappedSecondary(ev *fyne.PointEvent) {
	if i, _ := e.hitTest(ev.Position); i >= 0 {
		e.Rects = append(e.Rects[:i], e.Rects[i+1:]...)
		e.changed()
		e.Refresh()
	}
}

// --- desktop.Hoverable + desktop.Cursorable
func (e *CropEditor) MouseIn(ev *desktop.MouseEvent) { e.MouseMoved(ev) }
func (e *CropEditor) MouseOut()                      { e.cursor = desktop.DefaultCursor }
func (e *CropEditor) MouseMoved(ev *desktop.MouseEvent) {
	i, h := e.hitTest(ev.Position)
	switch {
	case i >= 0 && h >= 0:
		e.cursor = handleCursors[h]
	case i >= 0:
		e.cursor = desktop.PointerCursor
	default:
		e.cursor = desktop.CrosshairCursor
	}
}
func (e *CropEditor) Cursor() desktop.Cursor { return e.cursor }

// --- fyne.Focusable (keyboard)
func (e *CropEditor) FocusGained() {}
func (e *CropEditor) FocusLost()   {}
func (e *CropEditor) TypedRune(r rune) {
	switch r {
	case '+':
		e.forSelected(func(c *CropRect) { c.W += 2; c.H += 2 })
	case '-':
		e.forSelected(func(c *CropRect) { c.W -= 2; c.H -= 2 })
	}
}
func (e *CropEditor) TypedKey(k *fyne.KeyEvent) {
	switch k.Name {
	case fyne.KeyEscape:
		e.deselectAll()
	case fyne.KeyDelete, fyne.KeyBackspace:
		var keep []*CropRect
		for _, r := range e.Rects {
			if !r.Selected {
				keep = append(keep, r)
			}
		}
		e.Rects = keep
	case fyne.KeyLeft:
		e.forSelected(func(c *CropRect) { c.X-- })
	case fyne.KeyRight:
		e.forSelected(func(c *CropRect) { c.X++ })
	case fyne.KeyUp:
		e.forSelected(func(c *CropRect) { c.Y-- })
	case fyne.KeyDown:
		e.forSelected(func(c *CropRect) { c.Y++ })
	default:
		return
	}
	e.changed()
	e.Refresh()
}

func (e *CropEditor) forSelected(f func(*CropRect)) {
	for _, r := range e.Rects {
		if r.Selected {
			f(r)
			e.clamp(r)
		}
	}
	e.changed()
	e.Refresh()
}

func (e *CropEditor) deselectAll() {
	for _, r := range e.Rects {
		r.Selected = false
	}
}

func (e *CropEditor) changed() {
	if e.OnChange != nil {
		e.OnChange()
	}
}

// --- renderer
func (e *CropEditor) CreateRenderer() fyne.WidgetRenderer {
	im := canvas.NewImageFromImage(e.img)
	im.FillMode = canvas.ImageFillContain
	im.ScaleMode = canvas.ImageScaleFastest
	r := &editorRenderer{e: e, img: im}
	r.rebuild()
	return r
}

type editorRenderer struct {
	e    *CropEditor
	img  *canvas.Image
	objs []fyne.CanvasObject
}

func (r *editorRenderer) Layout(size fyne.Size) {
	r.img.Resize(size)
	r.rebuild()
}
func (r *editorRenderer) MinSize() fyne.Size { return r.e.imageSize() }
func (r *editorRenderer) Refresh() {
	r.rebuild()
	canvas.Refresh(r.e)
}
func (r *editorRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *editorRenderer) Destroy()                     {}

var (
	colRect     = color.NRGBA{R: 30, G: 110, B: 220, A: 255}
	colSelected = color.NRGBA{R: 220, G: 40, B: 40, A: 255}
	colFill     = color.NRGBA{R: 30, G: 110, B: 220, A: 28}
	colGhost    = color.NRGBA{R: 120, G: 120, B: 120, A: 120}
	colHandle   = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
)

// Frame statistics for the desktop frame-rate check (PLAN.md section 7.3):
// with BRISS_FRAME_STATS=1 the editor prints the median and 95th percentile of
// the interval between renderer rebuilds every 60 rebuilds. Dragging rebuilds
// on every pointer event, so the interval approximates the frame interval.
var frameStats struct {
	sync.Mutex
	enabled bool
	last    time.Time
	samples []time.Duration
}

func init() { frameStats.enabled = os.Getenv("BRISS_FRAME_STATS") != "" }

func recordFrame() {
	if !frameStats.enabled {
		return
	}
	frameStats.Lock()
	defer frameStats.Unlock()
	now := time.Now()
	if !frameStats.last.IsZero() {
		if d := now.Sub(frameStats.last); d < time.Second { // ignore idle gaps
			frameStats.samples = append(frameStats.samples, d)
		}
	}
	frameStats.last = now
	if len(frameStats.samples) >= 60 {
		sort.Slice(frameStats.samples, func(i, j int) bool { return frameStats.samples[i] < frameStats.samples[j] })
		p50 := frameStats.samples[len(frameStats.samples)/2]
		p95 := frameStats.samples[len(frameStats.samples)*95/100]
		fmt.Printf("frame interval over %d redraws: p50=%v p95=%v (%s)\n", len(frameStats.samples), p50.Round(100*time.Microsecond), p95.Round(100*time.Microsecond), map[bool]string{true: "PASS <16ms", false: "FAIL >=16ms"}[p95 < 16*time.Millisecond])
		frameStats.samples = frameStats.samples[:0]
	}
}

// Paint statistics: a 1x1 raster is added on every rebuild; Fyne calls its
// generator when it actually paints the frame, so the interval between calls
// is the painted-frame interval (bounded below by Fyne's 60 Hz draw ticker).
var paintStats struct {
	sync.Mutex
	last    time.Time
	samples []time.Duration
}

func recordPaint() {
	if !frameStats.enabled {
		return
	}
	paintStats.Lock()
	defer paintStats.Unlock()
	now := time.Now()
	if !paintStats.last.IsZero() {
		if d := now.Sub(paintStats.last); d < time.Second {
			paintStats.samples = append(paintStats.samples, d)
		}
	}
	paintStats.last = now
	if len(paintStats.samples) >= 60 {
		sort.Slice(paintStats.samples, func(i, j int) bool { return paintStats.samples[i] < paintStats.samples[j] })
		p50 := paintStats.samples[len(paintStats.samples)/2]
		p95 := paintStats.samples[len(paintStats.samples)*95/100]
		fmt.Printf("paint interval over %d frames: p50=%v p95=%v (%s)\n", len(paintStats.samples), p50.Round(100*time.Microsecond), p95.Round(100*time.Microsecond), map[bool]string{true: "PASS <=20ms", false: "FAIL >20ms"}[p95 <= 20*time.Millisecond])
		paintStats.samples = paintStats.samples[:0]
	}
}

var onePixel = image.NewNRGBA(image.Rect(0, 0, 1, 1))

func (r *editorRenderer) rebuild() {
	r.e.frameCount++
	recordFrame()
	objs := []fyne.CanvasObject{r.img}
	if frameStats.enabled {
		probe := canvas.NewRaster(func(w, h int) image.Image { recordPaint(); return onePixel })
		probe.Resize(fyne.NewSize(1, 1))
		objs = append(objs, probe)
	}
	if g := r.e.ghost; g != nil && r.e.mode == modeNone {
		gr := canvas.NewRectangle(color.Transparent)
		gr.StrokeColor, gr.StrokeWidth = colGhost, 1
		gr.Move(fyne.NewPos(g.X, g.Y))
		gr.Resize(fyne.NewSize(g.W, g.H))
		objs = append(objs, gr)
	}
	for i, c := range r.e.Rects {
		stroke := colRect
		if c.Selected {
			stroke = colSelected
		}
		if c.W < 5 || c.H < 5 {
			stroke = colSelected
		}
		rc := canvas.NewRectangle(colFill)
		rc.StrokeColor, rc.StrokeWidth = stroke, 2
		rc.Move(fyne.NewPos(c.X, c.Y))
		rc.Resize(fyne.NewSize(c.W, c.H))
		objs = append(objs, rc)
		for _, hp := range handlePositions(c) {
			h := canvas.NewRectangle(colHandle)
			h.StrokeColor, h.StrokeWidth = stroke, 1
			h.Move(fyne.NewPos(hp.X-handleSize/2, hp.Y-handleSize/2))
			h.Resize(fyne.NewSize(handleSize, handleSize))
			objs = append(objs, h)
		}
		t := canvas.NewText(fmt.Sprintf(" %d  %.0f x %.0f ", i, c.W, c.H), color.White)
		t.TextSize = 11
		t.Move(fyne.NewPos(c.X+3, c.Y+3))
		bg := canvas.NewRectangle(stroke)
		bg.Move(t.Position())
		bg.Resize(t.MinSize())
		objs = append(objs, bg, t)
	}
	r.objs = objs
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
