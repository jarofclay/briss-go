//go:build !darwin && !windows

package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"github.com/rymdport/portal"
	"github.com/rymdport/portal/filechooser"
)

// openSystemPicker asks the desktop's file chooser (GNOME, KDE and others,
// through the XDG desktop portal) for PDFs. Without the portal the answer is
// errNoSystemPicker and Fyne's own dialog stands in.
func openSystemPicker(w fyne.Window, done func([]string, error)) {
	parent := ""
	if nw, ok := w.(driver.NativeWindow); ok {
		nw.RunNative(func(ctx any) {
			if c, ok := ctx.(driver.X11WindowContext); ok && c.WindowHandle != 0 {
				parent = portal.FormatX11WindowHandle(c.WindowHandle)
			}
		})
	}
	go func() {
		uris, err := portalOpen(parent)
		fyne.Do(func() {
			if err != nil {
				done(nil, fmt.Errorf("%w: %v", errNoSystemPicker, err))
				return
			}
			done(fileURIPaths(uris), nil)
		})
	}()
}

func portalOpen(parent string) (uris []string, err error) {
	defer func() { // the portal library assumes well-formed answers
		if r := recover(); r != nil {
			err = fmt.Errorf("file chooser portal: %v", r)
		}
	}()
	return filechooser.OpenFile(parent, "Choose PDFs", &filechooser.OpenFileOptions{
		AcceptLabel: "Open",
		Multiple:    true,
		Filters: []*filechooser.Filter{{Name: "PDF files", Rules: []filechooser.Rule{
			{Type: filechooser.MIMEType, Pattern: "application/pdf"},
			{Type: filechooser.GlobPattern, Pattern: "*.pdf"},
			{Type: filechooser.GlobPattern, Pattern: "*.PDF"},
		}}},
	})
}
