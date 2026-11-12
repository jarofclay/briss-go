//go:build windows

package main

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"github.com/ncruces/zenity"
)

// openSystemPicker shows the Windows Open dialog (the Explorer one), attached
// to the Briss window. It runs on its own thread, so the window keeps drawing.
func openSystemPicker(w fyne.Window, done func([]string, error)) {
	opts := []zenity.Option{
		zenity.Title("Choose PDFs"),
		zenity.FileFilter{Name: "PDF files", Patterns: []string{"*.pdf"}, CaseFold: true},
	}
	if nw, ok := w.(driver.NativeWindow); ok {
		nw.RunNative(func(ctx any) {
			if c, ok := ctx.(driver.WindowsWindowContext); ok && c.HWND != 0 {
				opts = append(opts, zenity.Attach(c.HWND))
			}
		})
	}
	go func() {
		paths, err := zenity.SelectFileMultiple(opts...)
		if errors.Is(err, zenity.ErrCanceled) {
			paths, err = nil, nil
		}
		fyne.Do(func() { done(paths, err) })
	}()
}
