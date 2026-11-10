package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/jarofclay/briss-go/internal/app"
	"github.com/jarofclay/briss-go/internal/pdf"
)

type jobState int

const (
	stateQueued jobState = iota
	stateWorking
	stateReview // waiting in the preview window
	stateDone
	stateFailed
	stateSkipped
	stateLocked // waiting for a password
)

// Job is one dropped PDF.
type Job struct {
	Src, Dst string
	state    jobState
	caption  string
	analysis *app.Analysis
	settings Settings
	err      error  // why it failed, in full
	password string // for a protected PDF, as the user typed it
	wrong    bool   // the last password was refused
	row      *jobRow
}

// backend owns the PDF engine, started once and shared by all jobs.
type backend struct {
	once sync.Once
	eng  *pdf.Engine
	err  error
}

func (b *backend) engine() (*pdf.Engine, error) {
	b.once.Do(func() { b.eng, b.err = pdf.NewEngine(pdf.EngineOptions{}) })
	return b.eng, b.err
}

// runner processes jobs one after another in the background.
type runner struct {
	be      *backend
	queue   chan *Job
	preview func(*Job)       // opens the preview window (called on the UI goroutine)
	locked  func(*Job, bool) // asks for a password (called on the UI goroutine; true: the last one was wrong)
}

func newRunner(be *backend, preview func(*Job)) *runner {
	r := &runner{be: be, queue: make(chan *Job, 256), preview: preview}
	go r.loop()
	return r
}

func (r *runner) add(j *Job) { r.queue <- j }

func (r *runner) loop() {
	for j := range r.queue {
		r.run(j)
	}
}

// progress maps the core's stages onto the row's line.
func (j *Job) progress(stage string, done, total int) {
	fyne.Do(func() {
		switch stage {
		case "detect":
			j.row.set(stateWorking, "Finding pages", 0, true)
		case "ocr":
			f := float32(0.1)
			if total > 0 {
				f = 0.1 + 0.8*float32(done)/float32(total)
			}
			j.row.set(stateWorking, fmt.Sprintf("Reading text %d / %d", done, total), f, false)
		case "write":
			j.row.set(stateWorking, "Writing", 0.95, done == 0)
		}
	})
}

func (r *runner) run(j *Job) {
	eng, err := r.be.engine()
	if err != nil {
		j.fail(err)
		return
	}
	opts, ex := j.settings.options()
	opts.Password = j.password
	if j.settings.Mode == modeAuto {
		rep, err := app.Crop(context.Background(), eng, app.CropRequest{Src: j.Src, Dst: j.Dst, Options: opts, Extras: ex, Progress: j.progress})
		if r.needsPassword(j, err) {
			return
		}
		if err != nil {
			j.fail(err)
			return
		}
		j.finish(summary(rep))
		return
	}
	j.progress("detect", 0, 1)
	a, err := app.Analyze(context.Background(), eng, j.Src, opts)
	if r.needsPassword(j, err) {
		return
	}
	if err != nil {
		j.fail(err)
		return
	}
	j.analysis = a
	fyne.Do(func() {
		j.state = stateReview
		j.row.set(stateReview, "In preview", 0, false)
		r.preview(j)
	})
}

// needsPassword hands a protected PDF back to the user for its password.
func (r *runner) needsPassword(j *Job, err error) bool {
	if !errors.Is(err, pdf.ErrPasswordRequired) && !errors.Is(err, pdf.ErrBadPassword) {
		return false
	}
	wrong := errors.Is(err, pdf.ErrBadPassword)
	fyne.Do(func() {
		j.state, j.wrong = stateLocked, wrong
		caption := "Password needed"
		if wrong {
			caption = "Wrong password"
		}
		j.row.set(stateLocked, caption, 0, false)
		if r.locked != nil {
			r.locked(j, wrong)
		}
	})
	return true
}

func summary(rep *app.Report) string {
	parts := []string{plural(rep.OutputPages, "page")}
	if rep.Turned > 0 {
		parts = append(parts, fmt.Sprintf("%d turned", rep.Turned))
	}
	if rep.Straightened > 0 {
		parts = append(parts, fmt.Sprintf("%d straightened", rep.Straightened))
	}
	if rep.OCRPages > 0 {
		parts = append(parts, "text added")
	}
	for _, w := range rep.Warnings {
		if strings.Contains(w, "Tesseract") {
			parts = append(parts, "no OCR: Tesseract missing")
		}
	}
	return strings.Join(parts, " · ")
}

func (j *Job) finish(caption string) {
	fyne.Do(func() {
		j.state = stateDone
		j.row.set(stateDone, caption, 1, false)
	})
}

func (j *Job) fail(err error) {
	j.err = err
	msg := err.Error()
	switch {
	case errors.Is(err, pdf.ErrPasswordRequired):
		msg = "Password protected"
	case errors.Is(err, app.ErrInput):
		msg = strings.TrimPrefix(msg, "input: ")
	}
	if len(msg) > 90 {
		msg = msg[:87] + "…"
	}
	fyne.Do(func() {
		j.state = stateFailed
		j.row.set(stateFailed, msg, 1, false)
	})
}

// ---- the row in the job list ----

type jobRow struct {
	box      *fyne.Container
	name     *widget.Label
	caption  *canvas.Text
	status   *fyne.Container
	spinner  *widget.Activity
	doneIcon *canvas.Image
	line     *Line
	actions  *fyne.Container
	onOpen   func()
	onView   func()
	unlock   *IconButton
	view     *IconButton
	open     *IconButton
}

func newJobRow(j *Job, onOpen, onView, onUnlock func()) *jobRow {
	bg := canvas.NewRectangle(colSurface)
	bg.CornerRadius = 12
	fi := canvas.NewImageFromResource(icon("file", colDim))
	fi.FillMode = canvas.ImageFillContain
	fi.SetMinSize(fyne.NewSize(20, 20))
	name := widget.NewLabel(filepath.Base(j.Src))
	name.Truncation = fyne.TextTruncateEllipsis
	cap := canvas.NewText("Waiting", colDim)
	cap.TextSize = 11
	r := &jobRow{name: name, caption: cap, line: NewLine(), onOpen: onOpen, onView: onView}
	r.spinner = widget.NewActivity()
	r.doneIcon = canvas.NewImageFromResource(icon("check", colSuccess))
	r.doneIcon.FillMode = canvas.ImageFillContain
	r.doneIcon.SetMinSize(fyne.NewSize(20, 20))
	r.status = container.NewStack(r.spinner, r.doneIcon)
	r.view = NewIconButton("eye", 16, onView)
	r.open = NewIconButton("open", 16, onOpen)
	r.unlock = NewIconButton("lock", 16, onUnlock)
	r.unlock.Accent = true
	r.actions = container.NewHBox(r.view, r.open, r.unlock)
	inset := theme.InnerPadding()
	capBox := container.New(layout.NewCustomPaddedLayout(-4, 0, inset, 0), cap)
	textBox := container.NewVBox(name, capBox)
	right := container.NewHBox(r.actions, container.NewCenter(r.status))
	inner := container.NewBorder(nil, r.line, container.NewCenter(fi), right, textBox)
	r.box = container.NewStack(bg, container.NewPadded(container.NewPadded(inner)))
	r.set(stateQueued, "Waiting", 0, false)
	return r
}

func (r *jobRow) set(st jobState, caption string, value float32, busy bool) {
	r.caption.Text = caption
	r.caption.Color = colDim
	r.view.Hide()
	r.open.Hide()
	r.unlock.Hide()
	r.spinner.Hide()
	r.spinner.Stop()
	r.doneIcon.Hide()
	tint := colAccent
	switch st {
	case stateQueued:
		r.line.Hide()
	case stateWorking:
		r.line.Show()
		r.spinner.Show()
		r.spinner.Start()
	case stateReview:
		r.line.Hide()
		r.view.Show()
	case stateDone:
		r.line.Hide()
		r.open.Show()
		r.doneIcon.Resource = icon("check", colSuccess)
		r.doneIcon.Show()
	case stateFailed:
		r.line.Hide()
		r.caption.Color = colError
		r.doneIcon.Resource = icon("error", colError)
		r.doneIcon.Show()
	case stateSkipped:
		r.line.Hide()
		r.view.Show()
		r.doneIcon.Resource = icon("skip", colFaint)
		r.doneIcon.Show()
	case stateLocked:
		r.line.Hide()
		r.unlock.Show()
		if caption != "Password needed" {
			r.caption.Color = colError
		}
	}
	r.line.Set(value, busy, tint)
	r.caption.Refresh()
	r.doneIcon.Refresh()
	r.box.Refresh()
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
