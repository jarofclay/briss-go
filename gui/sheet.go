package main

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/jarofclay/briss-go/internal/geom"
)

type dragMode int

const (
	dragNone dragMode = iota
	dragMove
	dragResize
	dragDraw
)

// SheetView shows a page image with its crop rectangles: the parts that are
// kept bright, the rest dimmed. Drag inside a rectangle to move it, drag a
// handle to resize it, drag on empty space to draw a new one, right-click
// to remove one. Rectangles are briss ratios of the page.
type SheetView struct {
	widget.BaseWidget
	img       *image.Gray
	dim       *image.NRGBA
	rects     []geom.Ratios
	sel       int
	OnChanged func([]geom.Ratios)

	mode   dragMode
	handle int
	start  fyne.Position
	orig   geom.Ratios
	active int
}

func NewSheetView(changed func([]geom.Ratios)) *SheetView {
	v := &SheetView{sel: -1, active: -1, OnChanged: changed}
	v.ExtendBaseWidget(v)
	return v
}

// SetPage shows a new page image with its rectangles.
func (v *SheetView) SetPage(img *image.Gray, rects []geom.Ratios) {
	v.img = img
	v.dim = nil
	if img != nil {
		b := img.Bounds()
		v.dim = image.NewNRGBA(b)
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				g := img.Pix[y*img.Stride+x]
				l := uint8(int(g) * 30 / 100)
				i := y*v.dim.Stride + 4*x
				v.dim.Pix[i], v.dim.Pix[i+1], v.dim.Pix[i+2], v.dim.Pix[i+3] = l, l, l+4, 0xff
			}
		}
	}
	v.rects = append([]geom.Ratios(nil), rects...)
	v.sel = -1
	v.Refresh()
}

// placement is where the image sits inside the widget (aspect kept).
func (v *SheetView) placement() (ox, oy, pw, ph float32) {
	sz := v.Size()
	if v.img == nil || sz.Width <= 0 || sz.Height <= 0 {
		return 0, 0, sz.Width, sz.Height
	}
	b := v.img.Bounds()
	s := min(sz.Width/float32(b.Dx()), sz.Height/float32(b.Dy()))
	pw, ph = float32(b.Dx())*s, float32(b.Dy())*s
	return (sz.Width - pw) / 2, (sz.Height - ph) / 2, pw, ph
}

func (v *SheetView) toWidget(r geom.Ratios) (x0, y0, x1, y1 float32) {
	ox, oy, pw, ph := v.placement()
	return ox + float32(r.L)*pw, oy + float32(r.T)*ph, ox + float32(1-r.R)*pw, oy + float32(1-r.B)*ph
}

func handles(x0, y0, x1, y1 float32) [8]fyne.Position {
	mx, my := (x0+x1)/2, (y0+y1)/2
	return [8]fyne.Position{{X: x0, Y: y0}, {X: mx, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: my}, {X: x1, Y: y1}, {X: mx, Y: y1}, {X: x0, Y: y1}, {X: x0, Y: my}}
}

const handleHit = 9

// hit finds what is under p: a rectangle and the handle being grabbed
// (0..7 clockwise from the top-left corner; edges count as their middle
// handle), or -1 for the inside. The selected rectangle wins.
func (v *SheetView) hit(p fyne.Position) (int, int) {
	order := make([]int, 0, len(v.rects))
	if v.sel >= 0 && v.sel < len(v.rects) {
		order = append(order, v.sel)
	}
	for i := len(v.rects) - 1; i >= 0; i-- {
		if i != v.sel {
			order = append(order, i)
		}
	}
	for _, i := range order {
		x0, y0, x1, y1 := v.toWidget(v.rects[i])
		for h, hp := range handles(x0, y0, x1, y1) {
			if abs32(p.X-hp.X) <= handleHit && abs32(p.Y-hp.Y) <= handleHit {
				return i, h
			}
		}
		inY := p.Y >= y0-handleHit && p.Y <= y1+handleHit
		inX := p.X >= x0-handleHit && p.X <= x1+handleHit
		switch {
		case inY && abs32(p.X-x0) <= handleHit:
			return i, 7
		case inY && abs32(p.X-x1) <= handleHit:
			return i, 3
		case inX && abs32(p.Y-y0) <= handleHit:
			return i, 1
		case inX && abs32(p.Y-y1) <= handleHit:
			return i, 5
		}
	}
	for _, i := range order {
		x0, y0, x1, y1 := v.toWidget(v.rects[i])
		if p.X >= x0 && p.X <= x1 && p.Y >= y0 && p.Y <= y1 {
			return i, -1
		}
	}
	return -1, -1
}

func abs32(f float32) float32 { return float32(math.Abs(float64(f))) }

func (v *SheetView) Cursor() desktop.Cursor { return desktop.CrosshairCursor }

func (v *SheetView) Tapped(e *fyne.PointEvent) {
	i, _ := v.hit(e.Position)
	v.sel = i
	v.Refresh()
}

func (v *SheetView) TappedSecondary(e *fyne.PointEvent) {
	if i, _ := v.hit(e.Position); i >= 0 {
		v.rects = append(v.rects[:i], v.rects[i+1:]...)
		v.sel = -1
		v.Refresh()
		v.changed()
	}
}

func (v *SheetView) changed() {
	if v.OnChanged != nil {
		v.OnChanged(append([]geom.Ratios(nil), v.rects...))
	}
}

func (v *SheetView) Dragged(e *fyne.DragEvent) {
	if v.img == nil {
		return
	}
	if v.mode == dragNone {
		v.start = e.Position.Subtract(fyne.NewPos(e.Dragged.DX, e.Dragged.DY))
		i, h := v.hit(v.start)
		switch {
		case i >= 0 && h >= 0:
			v.mode, v.active, v.handle = dragResize, i, h
		case i >= 0:
			v.mode, v.active = dragMove, i
		default:
			ox, oy, pw, ph := v.placement()
			fx, fy := float64((v.start.X-ox)/pw), float64((v.start.Y-oy)/ph)
			if fx < 0 || fy < 0 || fx > 1 || fy > 1 {
				return
			}
			v.rects = append(v.rects, geom.Ratios{L: fx, R: 1 - fx, T: fy, B: 1 - fy})
			v.mode, v.active, v.handle = dragDraw, len(v.rects)-1, 4
		}
		v.sel = v.active
		v.orig = v.rects[v.active]
	}
	_, _, pw, ph := v.placement()
	dx, dy := float64((e.Position.X-v.start.X)/pw), float64((e.Position.Y-v.start.Y)/ph)
	o := v.orig
	x0, y0, x1, y1 := o.L, o.T, 1-o.R, 1-o.B
	switch v.mode {
	case dragMove:
		dx = math.Max(-x0, math.Min(1-x1, dx))
		dy = math.Max(-y0, math.Min(1-y1, dy))
		x0, x1, y0, y1 = x0+dx, x1+dx, y0+dy, y1+dy
	case dragResize, dragDraw:
		switch v.handle {
		case 0:
			x0, y0 = x0+dx, y0+dy
		case 1:
			y0 += dy
		case 2:
			x1, y0 = x1+dx, y0+dy
		case 3:
			x1 += dx
		case 4:
			x1, y1 = x1+dx, y1+dy
		case 5:
			y1 += dy
		case 6:
			x0, y1 = x0+dx, y1+dy
		case 7:
			x0 += dx
		}
	}
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	c := func(f float64) float64 { return math.Max(0, math.Min(1, f)) }
	x0, x1, y0, y1 = c(x0), c(x1), c(y0), c(y1)
	v.rects[v.active] = geom.Ratios{L: x0, R: 1 - x1, T: y0, B: 1 - y1}
	v.Refresh()
}

func (v *SheetView) DragEnd() {
	if v.mode == dragNone {
		return
	}
	r := v.rects[v.active]
	if 1-r.L-r.R < 0.02 || 1-r.T-r.B < 0.02 { // a click-sized rectangle: drop it
		v.rects = append(v.rects[:v.active], v.rects[v.active+1:]...)
		v.sel = -1
	}
	v.mode, v.active = dragNone, -1
	v.Refresh()
	v.changed()
}

func (v *SheetView) CreateRenderer() fyne.WidgetRenderer {
	base := canvas.NewImageFromImage(image.NewGray(image.Rect(0, 0, 1, 1)))
	base.FillMode = canvas.ImageFillStretch
	return &sheetRenderer{v: v, base: base}
}

type sheetRenderer struct {
	v       *SheetView
	base    *canvas.Image
	bright  []*canvas.Image
	frames  []*canvas.Rectangle
	knobs   []*canvas.Rectangle
	objects []fyne.CanvasObject
}

func (r *sheetRenderer) Destroy()                     {}
func (r *sheetRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *sheetRenderer) MinSize() fyne.Size           { return fyne.NewSize(200, 200) }
func (r *sheetRenderer) Layout(fyne.Size)             { r.Refresh() }

func (r *sheetRenderer) Refresh() {
	v := r.v
	r.objects = r.objects[:0]
	if v.img == nil {
		return
	}
	ox, oy, pw, ph := v.placement()
	if r.base.Image != v.dim {
		r.base.Image = v.dim
	}
	r.base.Move(fyne.NewPos(ox, oy))
	r.base.Resize(fyne.NewSize(pw, ph))
	r.base.Refresh()
	r.objects = append(r.objects, r.base)
	for len(r.bright) < len(v.rects) {
		im := canvas.NewImageFromImage(v.img)
		im.FillMode = canvas.ImageFillStretch
		f := canvas.NewRectangle(color.Transparent)
		f.StrokeWidth = 1.5
		r.bright, r.frames = append(r.bright, im), append(r.frames, f)
	}
	b := v.img.Bounds()
	for i, rr := range v.rects {
		x0, y0, x1, y1 := v.toWidget(rr)
		px0, py0 := int(rr.L*float64(b.Dx())), int(rr.T*float64(b.Dy()))
		px1, py1 := int((1-rr.R)*float64(b.Dx())), int((1-rr.B)*float64(b.Dy()))
		if px1 > px0 && py1 > py0 {
			im := r.bright[i]
			im.Image = v.img.SubImage(image.Rect(px0, py0, px1, py1))
			im.Move(fyne.NewPos(x0, y0))
			im.Resize(fyne.NewSize(x1-x0, y1-y0))
			im.Refresh()
			r.objects = append(r.objects, im)
		}
		f := r.frames[i]
		f.StrokeColor = withAlpha(colAccent, 0xb0)
		if i == v.sel {
			f.StrokeColor = colAccent
		}
		f.Move(fyne.NewPos(x0, y0))
		f.Resize(fyne.NewSize(x1-x0, y1-y0))
		f.Refresh()
		r.objects = append(r.objects, f)
	}
	if v.sel >= 0 && v.sel < len(v.rects) {
		x0, y0, x1, y1 := v.toWidget(v.rects[v.sel])
		for len(r.knobs) < 8 {
			k := canvas.NewRectangle(colText)
			k.CornerRadius = 2
			r.knobs = append(r.knobs, k)
		}
		for i, hp := range handles(x0, y0, x1, y1) {
			k := r.knobs[i]
			k.FillColor = colText
			k.StrokeColor, k.StrokeWidth = colAccent, 1
			k.Resize(fyne.NewSize(8, 8))
			k.Move(fyne.NewPos(hp.X-4, hp.Y-4))
			k.Refresh()
			r.objects = append(r.objects, k)
		}
	}
}
