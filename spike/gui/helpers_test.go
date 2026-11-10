package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func desktopMouseEvent(p fyne.Position) desktop.MouseEvent {
	return desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: p}}
}
