package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
)

// errNoSystemPicker means the system offers no file picker Briss can use (a
// Linux desktop without the file chooser portal); Fyne's own dialog stands in.
var errNoSystemPicker = errors.New("no system file picker")

// chooseFiles asks for PDFs with the system's own file picker: the Open panel
// on macOS, the Explorer dialog on Windows and the desktop's file chooser
// (through the XDG portal) on Linux. More than one file can be chosen, and on
// macOS folders too.
func (u *ui) chooseFiles() {
	if u.picking {
		return
	}
	u.picking = true
	openSystemPicker(u.w, func(paths []string, err error) { // on the main goroutine
		u.picking = false
		switch {
		case errors.Is(err, errNoSystemPicker):
			log.Printf("briss: %v; using Fyne's file dialog", err)
			u.chooseFilesFyne()
		case err != nil:
			dialog.ShowError(fmt.Errorf("the file picker did not open (%v); drop the PDFs on the window instead", err), u.w)
		case len(paths) > 0:
			u.addPaths(paths)
		}
	})
}

// chooseFilesFyne is the fallback: Fyne's built-in dialog, one file at a time.
func (u *ui) chooseFilesFyne() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		path := r.URI().Path()
		r.Close()
		u.addPaths([]string{path})
	}, u.w)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".pdf", ".PDF"}))
	d.Resize(u.w.Canvas().Size())
	d.Show()
}

// splitNUL splits paths that each end in a NUL byte, as the macOS panel returns them.
func splitNUL(b []byte) []string {
	var paths []string
	for _, p := range bytes.Split(b, []byte{0}) {
		if len(p) > 0 {
			paths = append(paths, string(p))
		}
	}
	return paths
}

// fileURIPaths turns the file:// URIs of the portal (already unescaped) into
// paths, skipping anything that is not a local file.
func fileURIPaths(uris []string) []string {
	var paths []string
	for _, u := range uris {
		if p, ok := strings.CutPrefix(u, "file://"); ok && p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}
