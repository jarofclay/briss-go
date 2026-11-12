package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

// icon.png is the Briss logo at 512 pixels, rendered from assets/logo. The
// downloads also carry it as Briss.icns (macOS) and rsrc_windows_*.syso
// (Windows); this copy is the window and Dock icon while the app runs.
//
//go:embed icon.png
var iconPNG []byte

func appIcon() fyne.Resource { return fyne.NewStaticResource("briss.png", iconPNG) }
