package main

import (
	"github.com/jarofclay/briss-go/internal/cropdef"
	"github.com/jarofclay/briss-go/internal/geom"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	a := test.NewTempApp(t)
	s := loadSettings(a.Preferences())
	if s != defaultSettings() {
		t.Fatalf("defaults: %+v", s)
	}
	if !s.Normalize || !s.OCR || !s.Straighten || !s.AutoRotate || s.Mode != modeAuto {
		t.Fatalf("normalise, OCR, straighten and auto-rotate are on by default, mode auto: %+v", s)
	}
	s.Mode, s.MarginPt, s.OCR = modePreviewOnly, 12, false
	s.save(a.Preferences())
	if got := loadSettings(a.Preferences()); got != s {
		t.Fatalf("got %+v want %+v", got, s)
	}
}

func TestPdfsIn(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.pdf", "b.PDF", "a_cropped.pdf", "notes.txt"} {
		os.WriteFile(filepath.Join(dir, n), []byte("%PDF"), 0o644)
	}
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "c.pdf"), []byte("%PDF"), 0o644)
	got := pdfsIn([]string{dir})
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
}

// waitFor polls cond on the UI goroutine until it holds or time runs out.
func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		ok := false
		fyne.DoAndWait(func() { ok = cond() })
		if ok {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out")
}

// Dropping a PDF in automatic mode writes NAME_cropped.pdf next to it.
func TestAutoJobWritesOutput(t *testing.T) {
	a := test.NewTempApp(t)
	a.Settings().SetTheme(darkTheme{})
	u := newUI(a)
	src := filepath.Join(t.TempDir(), "book.pdf")
	copyFile(t, "../testdata/pdfs/example.pdf", src)
	u.settings.OCR = false
	fyne.DoAndWait(func() { u.addPaths([]string{src}) })
	waitFor(t, 60*time.Second, func() bool { return u.jobs[0].state == stateDone || u.jobs[0].state == stateFailed })
	if j := u.jobs[0]; j.state != stateDone {
		t.Fatalf("job failed: %v", j.err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(src), "book_cropped.pdf")); err != nil {
		t.Fatal(err)
	}
}

// A protected PDF waits for its password: a wrong one asks again, the
// right one crops it.
func TestPasswordPrompt(t *testing.T) {
	a := test.NewTempApp(t)
	a.Settings().SetTheme(darkTheme{})
	u := newUI(a)
	u.settings.OCR = false
	src := filepath.Join(t.TempDir(), "secret.pdf")
	copyFile(t, "../testdata/pdfs/example-protected.pdf", src)
	fyne.DoAndWait(func() { u.addPaths([]string{src}) })
	j := u.jobs[0]
	waitFor(t, 60*time.Second, func() bool { return j.state == stateLocked })
	fyne.DoAndWait(func() {
		if u.pwDialog == nil || u.pwJob != j || j.wrong || j.row.caption.Text != "Password needed" {
			t.Errorf("expected a password prompt for the file: dialog %v, wrong %v, caption %q", u.pwDialog != nil, j.wrong, j.row.caption.Text)
		}
		u.unlock(j, "not it")
	})
	waitFor(t, 60*time.Second, func() bool { return j.state == stateLocked && j.wrong })
	fyne.DoAndWait(func() {
		if u.pwDialog == nil || j.row.caption.Text != "Wrong password" {
			t.Errorf("a wrong password must ask again")
		}
		u.unlock(j, "secret")
	})
	waitFor(t, 60*time.Second, func() bool { return j.state == stateDone || j.state == stateFailed })
	if j.state != stateDone {
		t.Fatalf("job failed: %v", j.err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(src), "secret_cropped.pdf")); err != nil {
		t.Fatal(err)
	}
}

// TestScreenshots writes PNGs of the main screens to BRISS_GUI_SHOTS; with
// BRISS_GUI_SAMPLE=scan.pdf it also shows that scan in the preview.
func TestScreenshots(t *testing.T) {
	out := os.Getenv("BRISS_GUI_SHOTS")
	if out == "" {
		t.Skip("set BRISS_GUI_SHOTS")
	}
	os.MkdirAll(out, 0o755)
	a := test.NewTempApp(t)
	a.Settings().SetTheme(darkTheme{})
	shot := func(w fyne.Window, name string) {
		img := w.Canvas().Capture()
		f, err := os.Create(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, img)
		f.Close()
	}
	u := newUI(a)
	u.w.Resize(fyne.NewSize(560, 620))
	shot(u.w, "1-start.png")
	fyne.DoAndWait(u.toggleOptions)
	shot(u.w, "2-options.png")
	fyne.DoAndWait(u.toggleOptions)

	dir := t.TempDir()
	var srcs []string
	for i, n := range []string{"example.pdf", "CREATIVE_COMMONS.pdf", "book300.pdf"} {
		p := filepath.Join(dir, []string{"Collected Essays.pdf", "Field Notes 1923.pdf", "A Long Book.pdf"}[i])
		copyFile(t, "../testdata/pdfs/"+n, p)
		srcs = append(srcs, p)
	}
	sample := os.Getenv("BRISS_GUI_SAMPLE")
	if sample != "" {
		p := filepath.Join(dir, "Scanned Lectures.pdf")
		copyFile(t, sample, p)
		srcs = append(srcs, p)
	}
	u.settings.OCR = false
	fyne.DoAndWait(func() { u.addPaths(srcs) })
	waitFor(t, 120*time.Second, func() bool {
		for _, j := range u.jobs {
			if j.state != stateDone && j.state != stateFailed {
				return false
			}
		}
		return true
	})
	shot(u.w, "3-jobs.png")

	if sample != "" {
		u.settings.Mode = modePreview
		p := filepath.Join(dir, "Scanned Lectures (preview).pdf")
		copyFile(t, sample, p)
		fyne.DoAndWait(func() { u.addPaths([]string{p}) })
		var pw fyne.Window
		waitFor(t, 120*time.Second, func() bool {
			for _, w := range a.Driver().AllWindows() {
				if strings.Contains(w.Title(), "(preview)") {
					pw = w
				}
			}
			return pw != nil
		})
		time.Sleep(6 * time.Second) // page render and results
		fyne.DoAndWait(func() { pw.Resize(fyne.NewSize(1100, 760)) })
		time.Sleep(time.Second)
		shot(pw, "4-preview.png")
	}
}

// Boxes edited on a page shown upright are stored on the page as it is in
// the file, with the page's turn, and come back the same.
func TestPreviewEditsTurnedPage(t *testing.T) {
	def := &cropdef.Definition{Version: cropdef.Version, Source: cropdef.Source{Path: "x.pdf", Pages: 1},
		PageOverrides: map[int][]cropdef.Rect{1: {{L: 0.1, B: 0.2, R: 0.3, T: 0.05, Turn: 270}}}}
	s := &session{def: def}
	if s.pageTurn(1) != 270 {
		t.Fatalf("page turn %d", s.pageTurn(1))
	}
	shown := s.upright(s.rawRects(1), 270)
	edited := []geom.Ratios{shown[0], {L: 0.55, B: 0.1, R: 0.05, T: 0.1}} // a second page drawn on the right
	s.setRects(1, edited)
	back := s.upright(s.rawRects(1), s.pageTurn(1))
	for i := range edited {
		if math.Abs(back[i].L-edited[i].L) > 1e-9 || math.Abs(back[i].T-edited[i].T) > 1e-9 {
			t.Fatalf("box %d: %+v came back as %+v", i, edited[i], back[i])
		}
		if r := s.rawRects(1)[i]; r.Turn != 270 {
			t.Fatalf("box %d lost the turn: %+v", i, r)
		}
	}
}
