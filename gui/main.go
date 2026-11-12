// Command briss-gui is the desktop app of briss: drop PDFs, get them cropped
// for reading on small screens. Everything happens automatically unless the
// options panel says otherwise (preview first, margins, page sizes, OCR,
// straightening).
package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/jarofclay/briss-go/internal/app"
	"github.com/jarofclay/briss-go/internal/ocr"
)

// ui is the main window and its state.
type ui struct {
	a        fyne.App
	w        fyne.Window
	settings Settings
	be       *backend
	run      *runner
	jobs     []*Job
	jobsBox  *fyne.Container
	body     *fyne.Container
	drop     *DropZone
	panel    *fyne.Container
	optsBtn  *IconButton
	clearBtn *IconButton
	top      *fyne.Container
	hasOCR   bool

	pwDialog *dialog.FormDialog // the password prompt on screen, if any
	pwJob    *Job               // the file it asks for
	pwQueue  []*Job             // files waiting for their prompt
	picking  bool               // a file picker is open
}

func main() {
	a := fyneapp.NewWithID("io.github.jarofclay.briss")
	a.Settings().SetTheme(darkTheme{})
	a.SetIcon(appIcon())
	u := newUI(a)
	for _, arg := range os.Args[1:] { // files given on the command line or by "Open with"
		u.addPaths([]string{arg})
	}
	u.w.ShowAndRun()
}

func newUI(a fyne.App) *ui {
	u := &ui{a: a, w: a.NewWindow("Briss"), settings: loadSettings(a.Preferences()), be: &backend{}}
	_, err := ocr.Find()
	u.hasOCR = err == nil
	u.run = newRunner(u.be, u.openPreview)
	u.run.locked = func(j *Job, wrong bool) { u.askPassword(j, wrong) }
	u.w.SetContent(u.build())
	u.w.Resize(fyne.NewSize(560, 620))
	u.w.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		var paths []string
		for _, uri := range uris {
			paths = append(paths, uri.Path())
		}
		u.addPaths(paths)
	})
	go u.be.engine() // warm the PDF engine while the user picks files
	return u
}

func (u *ui) build() fyne.CanvasObject {
	mark := canvas.NewText("briss", colText)
	mark.TextSize = 20
	mark.TextStyle.Bold = true
	dot := canvas.NewCircle(colAccent)
	dotBox := container.NewWithoutLayout(dot)
	dot.Resize(fyne.NewSize(6, 6))
	dot.Move(fyne.NewPos(2, 15))
	dotBox.Resize(fyne.NewSize(10, 24))
	logo := canvas.NewImageFromResource(appIcon())
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(26, 26))
	brand := container.NewHBox(logo, mark, container.NewGridWrap(fyne.NewSize(10, 24), dotBox))
	u.optsBtn = NewIconButton("sliders", 18, u.toggleOptions)
	u.clearBtn = NewIconButton("clear", 18, u.clearFinished)
	u.clearBtn.Hide()
	header := container.NewBorder(nil, nil, brand, container.NewHBox(u.clearBtn, u.optsBtn))

	u.panel = u.optionsPanel()
	u.panel.Hide()

	u.drop = NewDropZone(u.chooseFiles)
	u.jobsBox = container.NewVBox()
	u.body = container.NewStack()
	u.layoutBody()
	u.top = container.NewVBox(header, u.panel)
	return container.NewPadded(container.NewPadded(container.NewBorder(u.top, nil, nil, nil, u.body)))
}

func (u *ui) layoutBody() {
	if len(u.jobs) == 0 {
		u.drop.SetCompact(false)
		u.body.Objects = []fyne.CanvasObject{container.NewPadded(u.drop)}
		u.clearBtn.Hide()
	} else {
		u.drop.SetCompact(true)
		list := container.NewVScroll(container.NewPadded(u.jobsBox))
		u.body.Objects = []fyne.CanvasObject{container.NewBorder(container.NewPadded(u.drop), nil, nil, nil, list)}
		u.clearBtn.Show()
	}
	u.body.Refresh()
	if u.top != nil {
		u.top.Refresh()
	}
}

func (u *ui) toggleOptions() {
	if u.panel.Visible() {
		u.panel.Hide()
		u.optsBtn.Accent = false
	} else {
		u.panel.Show()
		u.optsBtn.Accent = true
	}
	u.optsBtn.Refresh()
	u.top.Refresh()
	u.w.Content().Refresh()
}

// label is a quiet caption for the options panel.
func label(s string) *canvas.Text {
	t := canvas.NewText(s, colDim)
	t.TextSize = 12
	return t
}

func (u *ui) optionsPanel() *fyne.Container {
	s := &u.settings
	save := func() { s.save(u.a.Preferences()) }
	mode := NewSegmented([]string{"Auto", "Preview", "Preview only"}, s.Mode, func(i int) { s.Mode = i; save() })

	marginVal := label(fmt.Sprintf("%.0f pt", s.MarginPt))
	slider := widget.NewSlider(0, 36)
	slider.Step = 1
	slider.Value = s.MarginPt
	slider.OnChanged = func(v float64) { s.MarginPt = v; marginVal.Text = fmt.Sprintf("%.0f pt", v); marginVal.Refresh() }
	slider.OnChangeEnded = func(float64) { save() }

	toggle := func(v *bool) *Switch { return NewSwitch(*v, func(on bool) { *v = on; save() }) }
	ocrSwitch := toggle(&s.OCR)
	ocrLabel := fyne.CanvasObject(label("Selectable text (OCR)"))
	if !u.hasOCR {
		ocrSwitch.Disabled, ocrSwitch.On, ocrSwitch.knobX = true, false, 0
		hint := widget.NewHyperlink("install Tesseract", mustURL("https://tesseract-ocr.github.io/tessdoc/Installation.html"))
		hint.TextStyle = fyne.TextStyle{}
		ocrLabel = container.NewHBox(label("Selectable text (OCR) —"), hint)
	}
	row := func(l fyne.CanvasObject, c fyne.CanvasObject) fyne.CanvasObject {
		return container.NewBorder(nil, nil, container.NewCenter(l), c)
	}
	rows := container.NewVBox(
		row(label("Mode"), mode),
		row(label("Margin"), container.NewBorder(nil, nil, nil, marginVal, container.NewGridWrap(fyne.NewSize(180, 30), slider))),
		row(label("Same page size"), toggle(&s.Normalize)),
		row(ocrLabel, ocrSwitch),
		row(label("Straighten"), toggle(&s.Straighten)),
		row(label("Auto-rotate"), toggle(&s.AutoRotate)),
		row(label("Split two-page sheets"), toggle(&s.SplitSheets)),
		row(label("Split each page"), NewSegmented([]string{"Off", "Columns", "Rows"}, s.SplitPages, func(i int) { s.SplitPages = i; save() })),
		row(label("Keep headers & page numbers"), toggle(&s.KeepHeaders)),
	)
	bg := canvas.NewRectangle(colSurface)
	bg.CornerRadius = 14
	bg.StrokeColor, bg.StrokeWidth = colBorder, 1
	return container.NewPadded(container.NewStack(bg, container.NewPadded(container.NewPadded(rows))))
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

// pdfsIn expands folders and keeps PDFs that are not briss outputs.
func pdfsIn(paths []string) []string {
	var out []string
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			filepath.WalkDir(p, func(q string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() && isSourcePDF(q) {
					out = append(out, q)
				}
				return nil
			})
			continue
		}
		if isSourcePDF(p) {
			out = append(out, p)
		}
	}
	return out
}

func isSourcePDF(p string) bool {
	low := strings.ToLower(p)
	return strings.HasSuffix(low, ".pdf") && !strings.HasSuffix(low, "_cropped.pdf")
}

func (u *ui) addPaths(paths []string) {
	for _, p := range pdfsIn(paths) {
		dst, err := app.Destination(p, "")
		if err != nil {
			continue
		}
		j := &Job{Src: p, Dst: dst, settings: u.settings}
		j.row = newJobRow(j, func() { u.openFile(j.Dst) }, func() { u.openPreview(j) }, func() { u.askPassword(j, j.wrong) })
		u.jobs = append(u.jobs, j)
		u.jobsBox.Add(j.row.box)
		u.run.add(j)
	}
	u.layoutBody()
}

func (u *ui) clearFinished() {
	var keep []*Job
	u.jobsBox.Objects = nil
	for _, j := range u.jobs {
		if j.state == stateQueued || j.state == stateWorking || j.state == stateReview || j.state == stateLocked {
			keep = append(keep, j)
			u.jobsBox.Add(j.row.box)
		}
	}
	u.jobs = keep
	u.layoutBody()
}

func (u *ui) openFile(path string) {
	abs, _ := filepath.Abs(path)
	u.a.OpenURL(&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)})
}

// spacer keeps an import of layout for small fixed gaps.
var _ = layout.NewSpacer

// askPassword asks for the password of a protected PDF, one file at a time.
func (u *ui) askPassword(j *Job, wrong bool) {
	if j.state != stateLocked {
		return
	}
	if u.pwDialog != nil {
		if u.pwJob != j {
			for _, q := range u.pwQueue {
				if q == j {
					return
				}
			}
			u.pwQueue = append(u.pwQueue, j)
		}
		return
	}
	entry := widget.NewPasswordEntry()
	entry.SetPlaceHolder("Password")
	hint := "This PDF is protected."
	if wrong {
		hint = "That password did not open it. Try again."
	}
	items := []*widget.FormItem{widget.NewFormItem("", label(hint)), widget.NewFormItem("", entry)}
	d := dialog.NewForm(filepath.Base(j.Src), "Unlock", "Cancel", items, func(ok bool) {
		if ok && entry.Text != "" {
			u.unlock(j, entry.Text)
		}
	}, u.w)
	entry.OnSubmitted = func(string) { d.Submit() }
	d.SetOnClosed(func() {
		u.pwDialog, u.pwJob = nil, nil
		for len(u.pwQueue) > 0 {
			next := u.pwQueue[0]
			u.pwQueue = u.pwQueue[1:]
			if next.state == stateLocked {
				u.askPassword(next, next.wrong)
				return
			}
		}
	})
	d.Resize(fyne.NewSize(360, d.MinSize().Height))
	u.pwDialog, u.pwJob = d, j
	d.Show()
	u.w.Canvas().Focus(entry)
}

// unlock retries a protected PDF with a password.
func (u *ui) unlock(j *Job, password string) {
	if u.pwJob == j && u.pwDialog != nil {
		u.pwDialog.Hide()
	}
	j.password, j.state = password, stateQueued
	j.row.set(stateQueued, "Waiting", 0, false)
	u.run.add(j)
}
