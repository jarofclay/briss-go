package main

import (
	"context"
	"fmt"
	"image"
	"math"
	"path/filepath"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"github.com/jarofclay/briss-go/internal/app"
	"github.com/jarofclay/briss-go/internal/crop"
	"github.com/jarofclay/briss-go/internal/cropdef"
	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/jarofclay/briss-go/internal/pdf"
	"github.com/jarofclay/briss-go/internal/raster"
)

// sheetWidth is the render width of pages in the preview, in pixels.
const sheetWidth = 1400

// session is one PDF in the preview window: its crop definition (edited in
// place) and the pages the current settings would write.
type session struct {
	j      *Job
	eng    *pdf.Engine
	doc    *pdf.Document
	def    *cropdef.Definition
	ex     app.Extras
	page   int // 1-based
	mu     sync.Mutex
	sheets map[int]*image.Gray
	busy   map[int]bool
	outs   map[int][]crop.Output
}

func (s *session) outputs() error {
	outs, err := app.Outputs(s.def, s.doc.Pages, s.ex)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.outs = outs
	s.mu.Unlock()
	return nil
}

// rawRects are the rectangles of page p as the definition holds them.
func (s *session) rawRects(p int) []cropdef.Rect {
	po, err := s.def.PageOutputs()
	if err != nil {
		return nil
	}
	return po[p]
}

// setRects replaces page p's rectangles; a rectangle keeps the angle of the
// one it replaces, so straightening still knows the page's lean.
// The rectangles come as drawn on the page shown upright and are stored on
// the page as it is in the file, with the page's turn.
func (s *session) setRects(p int, rs []geom.Ratios) {
	old := s.rawRects(p)
	turn := s.pageTurn(p)
	out := make([]cropdef.Rect, len(rs))
	for i, r := range rs {
		out[i] = cropdef.FromRatios(r.Turn(360 - turn))
		out[i].Turn = turn
		if i < len(old) {
			out[i].Angle = old[i].Angle
		} else if len(old) > 0 {
			out[i].Angle = old[len(old)-1].Angle
		}
	}
	if s.def.PageOverrides == nil {
		s.def.PageOverrides = map[int][]cropdef.Rect{}
	}
	s.def.PageOverrides[p] = out
}

// pageTurn is the quarter turn that shows page p upright (the turn its
// rectangles carry; 0 when it has none).
func (s *session) pageTurn(p int) int {
	if rs := s.rawRects(p); len(rs) > 0 {
		return rs[0].Turn
	}
	return 0
}

// upright returns page p's rectangles as drawn on the page shown upright.
func (s *session) upright(rs []cropdef.Rect, turn int) []geom.Ratios {
	var out []geom.Ratios
	for _, r := range rs {
		out = append(out, r.Ratios().Turn(turn))
	}
	return out
}

// sheet renders page p (cached) and calls done on the UI goroutine.
func (s *session) sheet(p int, done func(*image.Gray)) {
	s.mu.Lock()
	if g, ok := s.sheets[p]; ok {
		s.mu.Unlock()
		if done != nil {
			done(g)
		}
		return
	}
	if s.busy[p] {
		s.mu.Unlock()
		return
	}
	s.busy[p] = true
	s.mu.Unlock()
	go func() {
		pi := s.doc.Pages[p-1]
		job := pdf.RenderJob{Page: p - 1, Width: sheetWidth, Height: max(1, int(math.Round(sheetWidth*pi.Height/pi.Width)))}
		var out *image.Gray
		s.eng.Render(context.Background(), s.doc, []pdf.RenderJob{job}, func(_ pdf.RenderJob, g *image.Gray) error {
			out = image.NewGray(g.Bounds())
			copy(out.Pix, g.Pix)
			return nil
		})
		s.mu.Lock()
		s.busy[p] = false
		if out != nil {
			s.sheets[p] = out
		}
		s.mu.Unlock()
		if out != nil && done != nil {
			fyne.Do(func() { done(out) })
		}
	}()
}

// result renders what output o of page p will look like: turned, then cut.
func result(g *image.Gray, o crop.Output) *image.Gray {
	b := g.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	src := g
	if o.Angle != 0 {
		px, py := o.PivotPoint()
		src = raster.Rotate(g, o.Angle, px*w, py*h, 255)
	}
	r := image.Rect(int(o.Rect.L*w), int(o.Rect.T*h), int((1-o.Rect.R)*w), int((1-o.Rect.B)*h))
	return raster.RotateQuarter(raster.Crop(src, r), o.Turn)
}

func (u *ui) openPreview(j *Job) {
	if j.analysis == nil {
		return
	}
	eng, err := u.be.engine()
	if err != nil {
		j.fail(err)
		return
	}
	_, ex := j.settings.options()
	s := &session{j: j, eng: eng, doc: j.analysis.Result.Doc, def: j.analysis.Def, ex: ex, page: 1,
		sheets: map[int]*image.Gray{}, busy: map[int]bool{}}
	if err := s.outputs(); err != nil {
		j.fail(err)
		return
	}
	w := u.a.NewWindow(filepath.Base(j.Src))
	n := len(s.doc.Pages)

	results := container.NewVBox()
	view := NewSheetView(nil)
	counter := label(fmt.Sprintf("%d / %d", n, n))
	counterBox := container.NewGridWrap(fyne.NewSize(counter.MinSize().Width+16, 30), container.NewCenter(counter))
	refreshResults := func() {
		p := s.page
		s.mu.Lock()
		g, outs := s.sheets[p], s.outs[p]
		s.mu.Unlock()
		results.Objects = nil
		if g == nil {
			results.Refresh()
			return
		}
		if len(outs) == 0 {
			outs = []crop.Output{{Rect: geom.Ratios{}}}
		}
		for _, o := range outs {
			img := canvas.NewImageFromImage(result(g, o))
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(190, 260))
			frame := canvas.NewRectangle(colSurface)
			frame.CornerRadius = 10
			frame.StrokeColor, frame.StrokeWidth = colBorder, 1
			results.Add(container.NewStack(frame, container.NewPadded(img)))
		}
		results.Refresh()
	}
	view.OnChanged = func(rs []geom.Ratios) {
		s.setRects(s.page, rs)
		if err := s.outputs(); err == nil {
			refreshResults()
		}
	}
	show := func(p int) {
		s.page = max(1, min(n, p))
		counter.Text = fmt.Sprintf("%d / %d", s.page, n)
		counter.Refresh()
		page := s.page
		s.sheet(page, func(g *image.Gray) {
			if s.page != page {
				return
			}
			turn := s.pageTurn(page)
			view.SetPage(raster.RotateQuarter(g, turn), s.upright(s.rawRects(page), turn))
			refreshResults()
		})
		for _, q := range []int{page + 1, page - 1} { // read ahead
			if q >= 1 && q <= n {
				s.sheet(q, nil)
			}
		}
	}

	prev := NewIconButton("left", 18, func() { show(s.page - 1) })
	next := NewIconButton("right", 18, func() { show(s.page + 1) })
	orig, _ := s.def.PageOutputs() // the detected rectangles, for Reset
	reset := NewIconButton("reset", 16, func() {
		if s.def.PageOverrides == nil {
			s.def.PageOverrides = map[int][]cropdef.Rect{}
		}
		s.def.PageOverrides[s.page] = append([]cropdef.Rect(nil), orig[s.page]...)
		if err := s.outputs(); err == nil {
			s.mu.Lock()
			g := s.sheets[s.page]
			s.mu.Unlock()
			if g != nil {
				turn := s.pageTurn(s.page)
				view.SetPage(raster.RotateQuarter(g, turn), s.upright(orig[s.page], turn))
			}
			refreshResults()
		}
	})
	saved := false
	var save *Pill
	if j.settings.Mode != modePreviewOnly {
		save = NewPill("Save", func() {
			if save.Disabled {
				return
			}
			save.Disabled, save.Text = true, "Saving…"
			save.Refresh()
			go u.saveSession(s, w, func(ok bool) { saved = ok })
		})
	}
	closeBtn := NewIconButton("close", 16, func() { w.Close() })
	name := label(filepath.Base(j.Src))
	right := container.NewHBox(reset)
	if save != nil {
		right.Add(container.NewCenter(save))
	}
	right.Add(closeBtn)
	top := container.NewBorder(nil, nil, container.NewCenter(name), right)
	nav := container.NewHBox(layout.NewSpacer(), prev, counterBox, next, layout.NewSpacer())
	split := container.NewHSplit(container.NewPadded(view), container.NewPadded(container.NewVScroll(results)))
	split.Offset = 0.72
	w.SetContent(container.NewPadded(container.NewBorder(top, nav, nil, nil, split)))
	w.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		switch e.Name {
		case fyne.KeyLeft, fyne.KeyPageUp:
			show(s.page - 1)
		case fyne.KeyRight, fyne.KeyPageDown, fyne.KeySpace:
			show(s.page + 1)
		case fyne.KeyEscape:
			w.Close()
		}
	})
	w.SetOnClosed(func() {
		if saved || j.state != stateReview {
			return
		}
		j.state = stateSkipped
		if j.settings.Mode == modePreviewOnly {
			j.row.set(stateSkipped, "Previewed", 0, false)
		} else {
			j.row.set(stateSkipped, "Not saved", 0, false)
		}
	})
	w.Resize(fyne.NewSize(1100, 760))
	show(1)
	w.Show()
}

// saveSession adds the text layer if asked and writes the cropped PDF.
func (u *ui) saveSession(s *session, w fyne.Window, done func(bool)) {
	j := s.j
	fyne.Do(func() { j.state = stateWorking })
	outs, err := app.Outputs(s.def, s.doc.Pages, s.ex)
	if err == nil && s.ex.OCR {
		_, _, err = app.AddOCR(context.Background(), s.eng, s.doc, outs, s.ex.OCRLang, j.progress)
	}
	var st crop.Stats
	if err == nil {
		j.progress("write", 0, 1)
		st, err = crop.WriteOutputs(context.Background(), j.Src, j.Dst, outs, crop.Options{Password: s.j.analysis.Result.Password()})
	}
	if err != nil {
		j.fail(err)
		fyne.Do(func() { done(false) })
		return
	}
	turned, upright := 0, 0
	for _, os := range outs {
		for _, o := range os {
			if o.Angle != 0 {
				turned++
			}
			if o.Turn != 0 {
				upright++
			}
		}
	}
	caption := plural(st.OutputPages, "page")
	if upright > 0 {
		caption += fmt.Sprintf(" · %d turned", upright)
	}
	if turned > 0 {
		caption += fmt.Sprintf(" · %d straightened", turned)
	}
	j.finish(caption)
	fyne.Do(func() {
		done(true)
		w.Close()
	})
}
