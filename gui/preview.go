package main

import (
	"context"
	"fmt"
	"image"
	"math"
	"path/filepath"
	"sort"
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
	orig   map[int][]cropdef.Rect // the rectangles as detected, for Reset
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

// result renders what output o of page p will look like: turned, cut, and
// set on its larger blank page when same-size output gave it one.
func result(g *image.Gray, o crop.Output) *image.Gray {
	b := g.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	src := g
	if o.Angle != 0 {
		px, py := o.PivotPoint()
		src = raster.Rotate(g, o.Angle, px*w, py*h, 255)
	}
	r := image.Rect(int(o.Rect.L*w), int(o.Rect.T*h), int((1-o.Rect.R)*w), int((1-o.Rect.B)*h))
	cut := raster.Crop(src, r)
	if bx := o.Box; bx != nil {
		pw, ph := int(math.Round((1-bx.L-bx.R)*w)), int(math.Round((1-bx.T-bx.B)*h))
		cut = raster.Pad(cut, pw, ph, int(math.Round((o.Rect.L-bx.L)*w)), int(math.Round((o.Rect.T-bx.T)*h)), 255)
	}
	return raster.RotateQuarter(cut, o.Turn)
}

// previewWin is the one preview window. It holds every file waiting for
// review, each with its own session (edits are kept while the window is
// open), and shows one file and one page at a time.
type previewWin struct {
	u        *ui
	w        fyne.Window
	docs     []*Job
	sessions map[*Job]*session
	cur      *session

	view                    *SheetView
	results                 *fyne.Container
	pageText, docText       *canvas.Text
	srcNote, outNote        *canvas.Text
	pageBox, top            *fyne.Container
	prevDoc, nextDoc        *IconButton
	save                    *Pill
	saving                  map[*Job]bool
	first, prev, next, last *IconButton
}

// openPreview hands a file whose crops are ready to the preview window,
// opening it if needed. A file already on screen keeps its place; the new one
// joins the files the window steps through.
func (u *ui) openPreview(j *Job) {
	if j.analysis == nil {
		return
	}
	if u.pv == nil {
		u.pv = u.newPreviewWin()
	}
	pv := u.pv
	pv.add(j)
	if pv.cur == nil {
		pv.showDoc(j)
	}
	if pv.cur == nil { // the file could not be shown (its row says why)
		pv.docs = pv.docs[:0]
		pv.w.Close()
		return
	}
	pv.w.Show()
}

// showPreview opens the preview on file j (the eye button of its row).
func (u *ui) showPreview(j *Job) {
	u.openPreview(j)
	if u.pv != nil && (u.pv.cur == nil || u.pv.cur.j != j) {
		u.pv.showDoc(j)
	}
	if u.pv != nil {
		u.pv.w.RequestFocus()
	}
}

// heading is a section title over the source page and over the output pages.
func heading(title string) (*fyne.Container, *canvas.Text) {
	t := canvas.NewText(title, colText)
	t.TextSize = 13
	t.TextStyle.Bold = true
	note := canvas.NewText("", colDim)
	note.TextSize = 12
	return container.NewHBox(t, note), note
}

func (u *ui) newPreviewWin() *previewWin {
	pv := &previewWin{u: u, sessions: map[*Job]*session{}, saving: map[*Job]bool{}}
	pv.w = u.a.NewWindow("Briss preview")
	pv.view = NewSheetView(nil)
	pv.results = container.NewVBox()
	pv.view.OnChanged = func(rs []geom.Ratios) {
		s := pv.cur
		if s == nil {
			return
		}
		s.setRects(s.page, rs)
		if err := s.outputs(); err == nil {
			pv.refreshResults()
		}
	}

	// files: the one on screen, and the way to the others
	pv.docText = canvas.NewText("", colText)
	pv.docText.TextSize = 13
	pv.prevDoc = NewIconButton("fileprev", 18, func() { pv.stepDoc(-1) })
	pv.nextDoc = NewIconButton("filenext", 18, func() { pv.stepDoc(1) })
	files := container.NewHBox(pv.prevDoc, container.NewCenter(pv.docText), pv.nextDoc)

	// pages
	pv.pageText = canvas.NewText("", colText)
	pv.pageText.TextSize = 13
	pv.pageBox = container.NewGridWrap(fyne.NewSize(110, 32), container.NewCenter(pv.pageText))
	pv.first = NewIconButton("first", 18, func() { pv.stepPage(-1 << 30) })
	pv.prev = NewIconButton("left", 18, func() { pv.stepPage(-1) })
	pv.next = NewIconButton("right", 18, func() { pv.stepPage(1) })
	pv.last = NewIconButton("last", 18, func() { pv.stepPage(1 << 30) })
	pages := container.NewHBox(pv.first, pv.prev, pv.pageBox, pv.next, pv.last)
	pagesBg := canvas.NewRectangle(colSurface)
	pagesBg.CornerRadius = 10
	pagesBg.StrokeColor, pagesBg.StrokeWidth = colBorder, 1
	pageNav := container.NewStack(pagesBg, container.NewPadded(pages))

	reset := NewIconButton("reset", 16, pv.resetPage)
	pv.save = NewPill("Save", pv.saveCurrent)
	closeBtn := NewIconButton("close", 16, func() { pv.w.Close() })
	actions := container.NewHBox(reset, container.NewCenter(pv.save), closeBtn)

	top := container.NewBorder(nil, nil, files, actions, container.NewCenter(pageNav))
	pv.top = top
	srcHead, srcNote := heading("Source")
	outHead, outNote := heading("Output")
	pv.srcNote, pv.outNote = srcNote, outNote
	source := container.NewBorder(container.NewPadded(srcHead), nil, nil, nil, pv.view)
	output := container.NewBorder(container.NewPadded(outHead), nil, nil, nil, container.NewVScroll(pv.results))
	split := container.NewHSplit(container.NewPadded(source), container.NewPadded(output))
	split.Offset = 0.72
	pv.w.SetContent(container.NewPadded(container.NewBorder(container.NewPadded(top), nil, nil, nil, split)))
	pv.w.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		if pv.cur == nil {
			return
		}
		switch e.Name {
		case fyne.KeyLeft, fyne.KeyPageUp:
			pv.stepPage(-1)
		case fyne.KeyRight, fyne.KeyPageDown, fyne.KeySpace:
			pv.stepPage(1)
		case fyne.KeyHome:
			pv.stepPage(-1 << 30)
		case fyne.KeyEnd:
			pv.stepPage(1 << 30)
		case fyne.KeyLeftBracket:
			pv.stepDoc(-1)
		case fyne.KeyRightBracket:
			pv.stepDoc(1)
		case fyne.KeyEscape:
			pv.w.Close()
		}
	})
	pv.w.SetOnClosed(func() {
		for _, j := range pv.docs {
			if j.state != stateReview {
				continue
			}
			j.state = stateSkipped
			if j.settings.Mode == modePreviewOnly {
				j.row.set(stateSkipped, "Previewed", 0, false)
			} else {
				j.row.set(stateSkipped, "Not saved", 0, false)
			}
		}
		if u.pv == pv {
			u.pv = nil
		}
	})
	pv.w.Resize(fyne.NewSize(1100, 760))
	return pv
}

// add puts j among the files of the window, in the order of the job list.
func (pv *previewWin) add(j *Job) {
	for _, d := range pv.docs {
		if d == j {
			pv.refreshDocNav()
			return
		}
	}
	if j.state == stateSkipped { // opened again from its row
		j.state = stateReview
		j.row.set(stateReview, "In preview", 0, false)
	}
	pv.docs = append(pv.docs, j)
	order := map[*Job]int{}
	for i, x := range pv.u.jobs {
		order[x] = i
	}
	sort.SliceStable(pv.docs, func(a, b int) bool { return order[pv.docs[a]] < order[pv.docs[b]] })
	pv.refreshDocNav()
}

func (pv *previewWin) index(j *Job) int {
	for i, d := range pv.docs {
		if d == j {
			return i
		}
	}
	return -1
}

func (pv *previewWin) refreshDocNav() {
	if pv.cur == nil {
		return
	}
	name := filepath.Base(pv.cur.j.Src)
	if len(name) > 48 {
		name = name[:45] + "…"
	}
	if n := len(pv.docs); n > 1 {
		i := pv.index(pv.cur.j)
		pv.docText.Text = fmt.Sprintf("%s  ·  file %d of %d", name, i+1, n)
		pv.prevDoc.Disabled, pv.nextDoc.Disabled = i == 0, i == n-1
		pv.prevDoc.Show()
		pv.nextDoc.Show()
	} else {
		pv.docText.Text = name
		pv.prevDoc.Hide()
		pv.nextDoc.Hide()
	}
	pv.prevDoc.Refresh()
	pv.nextDoc.Refresh()
	pv.top.Refresh() // the name's width changed
	pv.docText.Refresh()
	pv.refreshSave()
}

func (pv *previewWin) refreshSave() {
	j := pv.cur.j
	switch {
	case pv.saving[j]:
		pv.save.Disabled, pv.save.Text = true, "Saving…"
	case j.state == stateDone:
		pv.save.Disabled, pv.save.Text = false, "Save again"
	default:
		pv.save.Disabled, pv.save.Text = false, "Save"
	}
	pv.save.Refresh()
}

// stepDoc moves to the previous (-1) or next (+1) file.
func (pv *previewWin) stepDoc(d int) {
	if pv.cur == nil || len(pv.docs) < 2 {
		return
	}
	i := pv.index(pv.cur.j) + d
	if i < 0 || i >= len(pv.docs) {
		return
	}
	pv.showDoc(pv.docs[i])
}

// session returns the session of j, starting it on first use.
func (pv *previewWin) session(j *Job) (*session, error) {
	if s := pv.sessions[j]; s != nil {
		return s, nil
	}
	eng, err := pv.u.be.engine()
	if err != nil {
		return nil, err
	}
	_, ex := j.settings.options()
	s := &session{j: j, eng: eng, doc: j.analysis.Result.Doc, def: j.analysis.Def, ex: ex, page: 1,
		sheets: map[int]*image.Gray{}, busy: map[int]bool{}}
	s.orig, _ = s.def.PageOutputs() // the detected rectangles, for Reset
	if err := s.outputs(); err != nil {
		return nil, err
	}
	pv.sessions[j] = s
	return s, nil
}

// showDoc puts file j on screen at the page it was left on.
func (pv *previewWin) showDoc(j *Job) {
	s, err := pv.session(j)
	if err != nil {
		j.fail(err)
		return
	}
	pv.cur = s
	pv.w.SetTitle(filepath.Base(j.Src))
	pv.refreshDocNav()
	pv.showPage(s.page)
}

// stepPage moves d pages on (clamped to the file).
func (pv *previewWin) stepPage(d int) {
	if pv.cur != nil {
		pv.showPage(pv.cur.page + d)
	}
}

// showPage puts page p of the current file on screen and reads ahead.
func (pv *previewWin) showPage(p int) {
	s := pv.cur
	if s == nil {
		return
	}
	n := len(s.doc.Pages)
	s.page = max(1, min(n, p))
	for _, b := range []*IconButton{pv.first, pv.prev, pv.next, pv.last} {
		b.Disabled = (b == pv.first || b == pv.prev) && s.page == 1 || (b == pv.next || b == pv.last) && s.page == n
		b.Refresh()
	}
	// as wide as the longest page number needs, so the arrows stay put
	pv.pageText.Text = fmt.Sprintf("Page %d of %d", n, n)
	pv.pageBox.Layout = layout.NewGridWrapLayout(fyne.NewSize(pv.pageText.MinSize().Width+24, 32))
	pv.pageText.Text = fmt.Sprintf("Page %d of %d", s.page, n)
	pv.pageText.Refresh()
	pv.pageBox.Refresh()
	pv.srcNote.Text = "  drag a box's edge or corner to change the crop"
	pv.srcNote.Refresh()
	page := s.page
	s.sheet(page, func(g *image.Gray) {
		if pv.cur != s || s.page != page {
			return
		}
		turn := s.pageTurn(page)
		pv.view.SetPage(raster.RotateQuarter(g, turn), s.upright(s.rawRects(page), turn))
		pv.refreshResults()
	})
	for _, q := range []int{page + 1, page - 1} { // read ahead
		if q >= 1 && q <= n {
			s.sheet(q, nil)
		}
	}
}

func (pv *previewWin) refreshResults() {
	s := pv.cur
	p := s.page
	s.mu.Lock()
	g, outs := s.sheets[p], s.outs[p]
	s.mu.Unlock()
	pv.results.Objects = nil
	if g == nil {
		pv.results.Refresh()
		return
	}
	pv.outNote.Text = "  " + plural(len(outs), "page") + " from this page"
	if len(outs) == 0 {
		pv.outNote.Text = "  this page is kept whole"
		outs = []crop.Output{{Rect: geom.Ratios{}}}
	}
	pv.outNote.Refresh()
	for _, o := range outs {
		img := canvas.NewImageFromImage(result(g, o))
		img.FillMode = canvas.ImageFillContain
		img.SetMinSize(fyne.NewSize(190, 260))
		frame := canvas.NewRectangle(colSurface)
		frame.CornerRadius = 10
		frame.StrokeColor, frame.StrokeWidth = colBorder, 1
		pv.results.Add(container.NewStack(frame, container.NewPadded(img)))
	}
	pv.results.Refresh()
}

// resetPage puts the current page's boxes back as detected.
func (pv *previewWin) resetPage() {
	s := pv.cur
	if s == nil {
		return
	}
	if s.def.PageOverrides == nil {
		s.def.PageOverrides = map[int][]cropdef.Rect{}
	}
	s.def.PageOverrides[s.page] = append([]cropdef.Rect(nil), s.orig[s.page]...)
	if err := s.outputs(); err != nil {
		return
	}
	s.mu.Lock()
	g := s.sheets[s.page]
	s.mu.Unlock()
	if g != nil {
		turn := s.pageTurn(s.page)
		pv.view.SetPage(raster.RotateQuarter(g, turn), s.upright(s.orig[s.page], turn))
	}
	pv.refreshResults()
}

// saveCurrent writes the file on screen (in every mode, Preview only
// included), then moves on to the next file not yet saved, or closes the
// window when there is none.
func (pv *previewWin) saveCurrent() {
	s := pv.cur
	if s == nil || pv.saving[s.j] {
		return
	}
	j := s.j
	pv.saving[j] = true
	pv.refreshSave()
	go pv.u.saveSession(s, func(ok bool) {
		delete(pv.saving, j)
		if pv.cur == nil || pv.cur.j != j {
			return
		}
		pv.refreshSave()
		if !ok {
			return
		}
		for k := 1; k < len(pv.docs); k++ { // the next file still waiting
			d := pv.docs[(pv.index(j)+k)%len(pv.docs)]
			if d.state == stateReview {
				pv.showDoc(d)
				return
			}
		}
		pv.w.Close()
	})
}

// saveSession adds the text layer if asked and writes the cropped PDF.
// done runs on the UI goroutine.
func (u *ui) saveSession(s *session, done func(bool)) {
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
	fyne.Do(func() { done(true) })
}
