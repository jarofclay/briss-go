package main

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
)

// Line icons on a 24x24 grid, stroked in the colour they are asked for.
var iconPaths = map[string]string{
	"drop":     `<path d="M12 3 L12 14.5"/><path d="M7.5 10 L12 14.5 L16.5 10"/><path d="M4 14.5 L4 18 C4 19.1 4.9 20 6 20 L18 20 C19.1 20 20 19.1 20 18 L20 14.5"/>`,
	"file":     `<path d="M14 3 L7 3 C5.9 3 5 3.9 5 5 L5 19 C5 20.1 5.9 21 7 21 L17 21 C18.1 21 19 20.1 19 19 L19 8 Z"/><path d="M14 3 L14 8 L19 8"/>`,
	"sliders":  `<path d="M4 7 L14 7"/><path d="M18 7 L20 7"/><circle cx="16" cy="7" r="2"/><path d="M4 17 L6 17"/><path d="M10 17 L20 17"/><circle cx="8" cy="17" r="2"/>`,
	"check":    `<circle cx="12" cy="12" r="9"/><path d="M8 12.5 L11 15.5 L16.5 9.5"/>`,
	"error":    `<circle cx="12" cy="12" r="9"/><path d="M12 7.5 L12 13"/><path d="M12 16.5 L12 16.6"/>`,
	"eye":      `<path d="M2.5 12 C4.5 7.5 8 5.5 12 5.5 C16 5.5 19.5 7.5 21.5 12 C19.5 16.5 16 18.5 12 18.5 C8 18.5 4.5 16.5 2.5 12 Z"/><circle cx="12" cy="12" r="3"/>`,
	"open":     `<path d="M14 4 L20 4 L20 10"/><path d="M20 4 L11 13"/><path d="M18 14 L18 19 C18 19.6 17.6 20 17 20 L5 20 C4.4 20 4 19.6 4 19 L4 7 C4 6.4 4.4 6 5 6 L10 6"/>`,
	"left":     `<path d="M15 5 L8 12 L15 19"/>`,
	"first":    `<path d="M17 5 L10 12 L17 19"/><path d="M6.5 5 L6.5 19"/>`,
	"last":     `<path d="M7 5 L14 12 L7 19"/><path d="M17.5 5 L17.5 19"/>`,
	"fileprev": `<path d="M14 3 L7 3 C5.9 3 5 3.9 5 5 L5 19 C5 20.1 5.9 21 7 21 L17 21 C18.1 21 19 20.1 19 19 L19 8 Z"/><path d="M14 3 L14 8 L19 8"/><path d="M13.5 11 L10.5 14 L13.5 17"/>`,
	"filenext": `<path d="M14 3 L7 3 C5.9 3 5 3.9 5 5 L5 19 C5 20.1 5.9 21 7 21 L17 21 C18.1 21 19 20.1 19 19 L19 8 Z"/><path d="M14 3 L14 8 L19 8"/><path d="M10.5 11 L13.5 14 L10.5 17"/>`,
	"right":    `<path d="M9 5 L16 12 L9 19"/>`,
	"close":    `<path d="M6 6 L18 18"/><path d="M18 6 L6 18"/>`,
	"reset":    `<path d="M4 12 C4 7.6 7.6 4 12 4 C15 4 17.6 5.7 19 8.2"/><path d="M19.5 3.5 L19.5 8.5 L14.5 8.5"/><path d="M20 12 C20 16.4 16.4 20 12 20 C9 20 6.4 18.3 5 15.8"/>`,
	"skip":     `<circle cx="12" cy="12" r="9"/><path d="M8 12 L16 12"/>`,
	"lock":     `<rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11 L8 8 C8 5.8 9.8 4 12 4 C14.2 4 16 5.8 16 8 L16 11"/>`,
	"clear":    `<path d="M5 7 L19 7"/><path d="M10 7 L10 4.5 L14 4.5 L14 7"/><path d="M7 7 L8 20 L16 20 L17 7"/>`,
}

func hex(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// icon returns the named line icon stroked in c.
func icon(name string, c color.NRGBA) fyne.Resource {
	p, ok := iconPaths[name]
	if !ok {
		panic("unknown icon " + name)
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="` + hex(c) +
		`" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + p + `</svg>`
	return fyne.NewStaticResource(name+strings.TrimPrefix(hex(c), "#")+".svg", []byte(svg))
}
