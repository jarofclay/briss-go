package main

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// ---- DropZone: the big target on the start screen ----

// DropZone is a rounded panel that invites dropping PDFs; tapping it opens
// the file chooser. Compact, it becomes a slim bar above the job list.
type DropZone struct {
	widget.BaseWidget
	OnTapped func()
	compact  bool
	hovered  bool
}

func NewDropZone(onTapped func()) *DropZone {
	d := &DropZone{OnTapped: onTapped}
	d.ExtendBaseWidget(d)
	return d
}

func (d *DropZone) SetCompact(c bool) {
	if d.compact != c {
		d.compact = c
		d.Refresh()
	}
}

func (d *DropZone) Tapped(*fyne.PointEvent)        { d.OnTapped() }
func (d *DropZone) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (d *DropZone) MouseIn(*desktop.MouseEvent)    { d.hovered = true; d.Refresh() }
func (d *DropZone) MouseMoved(*desktop.MouseEvent) {}
func (d *DropZone) MouseOut()                      { d.hovered = false; d.Refresh() }

func (d *DropZone) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(colSurface)
	bg.CornerRadius = 18
	bg.StrokeWidth = 1
	img := canvas.NewImageFromResource(icon("drop", colDim))
	img.FillMode = canvas.ImageFillContain
	title := canvas.NewText("Drop PDFs", colText)
	title.TextSize = 17
	sub := canvas.NewText("or click to choose", colDim)
	sub.TextSize = 11
	r := &dropRenderer{d: d, bg: bg, img: img, title: title, sub: sub}
	r.Refresh()
	return r
}

type dropRenderer struct {
	d          *DropZone
	bg         *canvas.Rectangle
	img        *canvas.Image
	title, sub *canvas.Text
}

func (r *dropRenderer) Destroy() {}
func (r *dropRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.img, r.title, r.sub}
}

func (r *dropRenderer) MinSize() fyne.Size {
	if r.d.compact {
		return fyne.NewSize(240, 64)
	}
	return fyne.NewSize(320, 220)
}

func (r *dropRenderer) Layout(s fyne.Size) {
	r.bg.Resize(s)
	ts, ss := r.title.MinSize(), r.sub.MinSize()
	if r.d.compact {
		is := float32(26)
		total := is + 14 + max(ts.Width, ss.Width)
		x := (s.Width - total) / 2
		r.img.Resize(fyne.NewSize(is, is))
		r.img.Move(fyne.NewPos(x, (s.Height-is)/2))
		r.title.Move(fyne.NewPos(x+is+14, s.Height/2-ts.Height+2))
		r.sub.Move(fyne.NewPos(x+is+14, s.Height/2+1))
		return
	}
	is := float32(52)
	y := (s.Height - is - 18 - ts.Height - 4 - ss.Height) / 2
	r.img.Resize(fyne.NewSize(is, is))
	r.img.Move(fyne.NewPos((s.Width-is)/2, y))
	r.title.Move(fyne.NewPos((s.Width-ts.Width)/2, y+is+18))
	r.sub.Move(fyne.NewPos((s.Width-ss.Width)/2, y+is+18+ts.Height+4))
}

func (r *dropRenderer) Refresh() {
	if r.d.hovered {
		r.bg.FillColor, r.bg.StrokeColor = colRaised, withAlpha(colAccent, 0xc0)
		r.img.Resource = icon("drop", colAccent)
	} else {
		r.bg.FillColor, r.bg.StrokeColor = colSurface, colBorder
		r.img.Resource = icon("drop", colDim)
	}
	if r.d.compact {
		r.title.Text, r.title.TextSize = "Drop more PDFs", 14
	} else {
		r.title.Text, r.title.TextSize = "Drop PDFs", 17
	}
	r.bg.Refresh()
	r.img.Refresh()
	r.title.Refresh()
	r.sub.Refresh()
}

// ---- Switch: an iOS-style toggle ----

// Switch is a two-state toggle with a sliding knob.
type Switch struct {
	widget.BaseWidget
	On        bool
	Disabled  bool
	OnChanged func(bool)
	knobX     float32 // 0 (off) .. 1 (on), animated
}

func NewSwitch(on bool, changed func(bool)) *Switch {
	s := &Switch{On: on, OnChanged: changed}
	if on {
		s.knobX = 1
	}
	s.ExtendBaseWidget(s)
	return s
}

func (s *Switch) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (s *Switch) Tapped(*fyne.PointEvent) {
	if s.Disabled {
		return
	}
	s.SetOn(!s.On)
	if s.OnChanged != nil {
		s.OnChanged(s.On)
	}
}

// SetOn changes the state with a short slide.
func (s *Switch) SetOn(on bool) {
	if s.On == on {
		return
	}
	s.On = on
	from, to := s.knobX, float32(0)
	if on {
		to = 1
	}
	fyne.NewAnimation(140*time.Millisecond, func(f float32) {
		s.knobX = from + (to-from)*f
		s.Refresh()
	}).Start()
}

func (s *Switch) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(colBorder)
	track.CornerRadius = 10
	knob := canvas.NewCircle(colText)
	r := &switchRenderer{s: s, track: track, knob: knob}
	r.Refresh()
	return r
}

type switchRenderer struct {
	s     *Switch
	track *canvas.Rectangle
	knob  *canvas.Circle
}

func (r *switchRenderer) Destroy()                     {}
func (r *switchRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.track, r.knob} }
func (r *switchRenderer) MinSize() fyne.Size           { return fyne.NewSize(36, 20) }

func (r *switchRenderer) Layout(sz fyne.Size) {
	tw, th := float32(36), float32(20)
	y := (sz.Height - th) / 2
	x := sz.Width - tw
	r.track.Resize(fyne.NewSize(tw, th))
	r.track.Move(fyne.NewPos(x, y))
	k := th - 6
	r.knob.Resize(fyne.NewSize(k, k))
	r.knob.Move(fyne.NewPos(x+3+r.s.knobX*(tw-k-6), y+3))
}

func (r *switchRenderer) Refresh() {
	mix := func(a, b color.NRGBA, f float32) color.NRGBA {
		l := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*f) }
		return color.NRGBA{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), A: 0xff}
	}
	r.track.FillColor = mix(colBorder, colAccent, r.s.knobX)
	r.knob.FillColor = mix(colDim, rgb(0xff, 0xff, 0xff), r.s.knobX)
	if r.s.Disabled {
		r.track.FillColor, r.knob.FillColor = colSurface, colFaint
	}
	r.Layout(r.s.Size())
	r.track.Refresh()
	r.knob.Refresh()
}

// ---- Segmented: one choice out of a few short words ----

// Segmented is a row of mutually exclusive options in one rounded pill.
type Segmented struct {
	widget.BaseWidget
	Options   []string
	Selected  int
	OnChanged func(int)
}

func NewSegmented(options []string, selected int, changed func(int)) *Segmented {
	s := &Segmented{Options: options, Selected: selected, OnChanged: changed}
	s.ExtendBaseWidget(s)
	return s
}

func (s *Segmented) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (s *Segmented) Tapped(e *fyne.PointEvent) {
	if len(s.Options) == 0 {
		return
	}
	i := int(e.Position.X / (s.Size().Width / float32(len(s.Options))))
	i = max(0, min(len(s.Options)-1, i))
	if i != s.Selected {
		s.Selected = i
		s.Refresh()
		if s.OnChanged != nil {
			s.OnChanged(i)
		}
	}
}

func (s *Segmented) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(colSurface)
	bg.CornerRadius = 9
	bg.StrokeColor, bg.StrokeWidth = colBorder, 1
	sel := canvas.NewRectangle(colRaised)
	sel.CornerRadius = 7
	r := &segRenderer{s: s, bg: bg, sel: sel}
	for _, o := range s.Options {
		t := canvas.NewText(o, colDim)
		t.TextSize = 12
		r.labels = append(r.labels, t)
	}
	r.Refresh()
	return r
}

type segRenderer struct {
	s       *Segmented
	bg, sel *canvas.Rectangle
	labels  []*canvas.Text
}

func (r *segRenderer) Destroy() {}
func (r *segRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.bg, r.sel}
	for _, l := range r.labels {
		objs = append(objs, l)
	}
	return objs
}

func (r *segRenderer) MinSize() fyne.Size {
	w := float32(0)
	for _, l := range r.labels {
		w = max(w, l.MinSize().Width)
	}
	return fyne.NewSize((w+28)*float32(len(r.labels)), 30)
}

func (r *segRenderer) Layout(sz fyne.Size) {
	r.bg.Resize(sz)
	n := float32(len(r.labels))
	cw := sz.Width / n
	r.sel.Resize(fyne.NewSize(max(0, cw-6), max(0, sz.Height-6)))
	r.sel.Move(fyne.NewPos(float32(r.s.Selected)*cw+3, 3))
	for i, l := range r.labels {
		ms := l.MinSize()
		l.Move(fyne.NewPos(float32(i)*cw+(cw-ms.Width)/2, (sz.Height-ms.Height)/2))
	}
}

func (r *segRenderer) Refresh() {
	for i, l := range r.labels {
		l.Color = colDim
		if i == r.s.Selected {
			l.Color = colText
		}
		l.Refresh()
	}
	r.Layout(r.s.Size())
	r.bg.Refresh()
	r.sel.Refresh()
}

// ---- Line: a thin progress line ----

// Line is a 2-pixel progress bar; with Busy it shows a sliding segment.
type Line struct {
	widget.BaseWidget
	value float32
	busy  bool
	tint  color.NRGBA
	anim  *fyne.Animation
	phase float32
}

func NewLine() *Line {
	l := &Line{tint: colAccent}
	l.ExtendBaseWidget(l)
	return l
}

// Set shows a fraction done, or a moving segment when busy.
func (l *Line) Set(value float32, busy bool, tint color.NRGBA) {
	l.value, l.tint = value, tint
	if busy != l.busy {
		l.busy = busy
		if busy {
			l.anim = fyne.NewAnimation(1200*time.Millisecond, func(f float32) { l.phase = f; l.Refresh() })
			l.anim.RepeatCount = fyne.AnimationRepeatForever
			l.anim.Curve = fyne.AnimationLinear
			l.anim.Start()
		} else if l.anim != nil {
			l.anim.Stop()
		}
	}
	l.Refresh()
}

func (l *Line) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(colBorder)
	bar := canvas.NewRectangle(colAccent)
	track.CornerRadius, bar.CornerRadius = 1, 1
	r := &lineRenderer{l: l, track: track, bar: bar}
	return r
}

type lineRenderer struct {
	l          *Line
	track, bar *canvas.Rectangle
}

func (r *lineRenderer) Destroy()                     {}
func (r *lineRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.track, r.bar} }
func (r *lineRenderer) MinSize() fyne.Size           { return fyne.NewSize(20, 2) }

func (r *lineRenderer) Layout(sz fyne.Size) {
	r.track.Resize(fyne.NewSize(sz.Width, 2))
	r.track.Move(fyne.NewPos(0, (sz.Height-2)/2))
	if r.l.busy {
		seg := sz.Width * 0.25
		x := -seg + (sz.Width+seg)*r.l.phase
		x0, x1 := max(0, x), min(sz.Width, x+seg)
		r.bar.Resize(fyne.NewSize(max(0, x1-x0), 2))
		r.bar.Move(fyne.NewPos(x0, (sz.Height-2)/2))
		return
	}
	r.bar.Resize(fyne.NewSize(sz.Width*max(0, min(1, r.l.value)), 2))
	r.bar.Move(fyne.NewPos(0, (sz.Height-2)/2))
}

func (r *lineRenderer) Refresh() {
	r.bar.FillColor = r.l.tint
	r.Layout(r.l.Size())
	r.track.Refresh()
	r.bar.Refresh()
}

// ---- IconButton: a flat icon with a soft hover ----

// IconButton is a borderless icon that lights up under the pointer.
type IconButton struct {
	widget.BaseWidget
	name     string
	OnTapped func()
	hovered  bool
	size     float32
	Accent   bool // stroke in the accent colour
}

func NewIconButton(name string, size float32, tapped func()) *IconButton {
	b := &IconButton{name: name, OnTapped: tapped, size: size}
	b.ExtendBaseWidget(b)
	return b
}

func (b *IconButton) Tapped(*fyne.PointEvent) {
	if b.OnTapped != nil {
		b.OnTapped()
	}
}
func (b *IconButton) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (b *IconButton) MouseIn(*desktop.MouseEvent)    { b.hovered = true; b.Refresh() }
func (b *IconButton) MouseMoved(*desktop.MouseEvent) {}
func (b *IconButton) MouseOut()                      { b.hovered = false; b.Refresh() }
func (b *IconButton) SetIcon(name string)            { b.name = name; b.Refresh() }

func (b *IconButton) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 8
	img := canvas.NewImageFromResource(icon(b.name, colDim))
	img.FillMode = canvas.ImageFillContain
	r := &iconBtnRenderer{b: b, bg: bg, img: img}
	r.Refresh()
	return r
}

type iconBtnRenderer struct {
	b   *IconButton
	bg  *canvas.Rectangle
	img *canvas.Image
}

func (r *iconBtnRenderer) Destroy()                     {}
func (r *iconBtnRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.img} }
func (r *iconBtnRenderer) MinSize() fyne.Size           { return fyne.NewSize(r.b.size+14, r.b.size+14) }

func (r *iconBtnRenderer) Layout(sz fyne.Size) {
	r.bg.Resize(sz)
	is := r.b.size
	r.img.Resize(fyne.NewSize(is, is))
	r.img.Move(fyne.NewPos((sz.Width-is)/2, (sz.Height-is)/2))
}

func (r *iconBtnRenderer) Refresh() {
	c := colDim
	if r.b.Accent {
		c = colAccent
	}
	if r.b.hovered {
		r.bg.FillColor = withAlpha(colText, 0x12)
		if !r.b.Accent {
			c = colText
		}
	} else {
		r.bg.FillColor = color.Transparent
	}
	r.img.Resource = icon(r.b.name, c)
	r.bg.Refresh()
	r.img.Refresh()
}

// ---- Pill: a solid rounded button for the one main action ----

// Pill is the primary button: white on black, rounded.
type Pill struct {
	widget.BaseWidget
	Text     string
	OnTapped func()
	hovered  bool
	Disabled bool
}

func NewPill(text string, tapped func()) *Pill {
	p := &Pill{Text: text, OnTapped: tapped}
	p.ExtendBaseWidget(p)
	return p
}

func (p *Pill) Tapped(*fyne.PointEvent) {
	if !p.Disabled && p.OnTapped != nil {
		p.OnTapped()
	}
}
func (p *Pill) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (p *Pill) MouseIn(*desktop.MouseEvent)    { p.hovered = true; p.Refresh() }
func (p *Pill) MouseMoved(*desktop.MouseEvent) {}
func (p *Pill) MouseOut()                      { p.hovered = false; p.Refresh() }

func (p *Pill) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(colText)
	bg.CornerRadius = 15
	t := canvas.NewText(p.Text, colBackground)
	t.TextSize = 12
	t.TextStyle.Bold = true
	r := &pillRenderer{p: p, bg: bg, t: t}
	r.Refresh()
	return r
}

type pillRenderer struct {
	p  *Pill
	bg *canvas.Rectangle
	t  *canvas.Text
}

func (r *pillRenderer) Destroy()                     {}
func (r *pillRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.t} }
func (r *pillRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.t.MinSize().Width+36, 30)
}
func (r *pillRenderer) Layout(sz fyne.Size) {
	r.bg.Resize(sz)
	ts := r.t.MinSize()
	r.t.Move(fyne.NewPos((sz.Width-ts.Width)/2, (sz.Height-ts.Height)/2))
}
func (r *pillRenderer) Refresh() {
	r.t.Text = r.p.Text
	switch {
	case r.p.Disabled:
		r.bg.FillColor, r.t.Color = colRaised, colFaint
	case r.p.hovered:
		r.bg.FillColor, r.t.Color = rgb(0xff, 0xff, 0xff), colBackground
	default:
		r.bg.FillColor, r.t.Color = colText, colBackground
	}
	r.bg.Refresh()
	r.t.Refresh()
}
