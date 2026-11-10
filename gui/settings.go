package main

import (
	"fyne.io/fyne/v2"

	"github.com/jarofclay/briss-go/internal/app"
)

// Modes of the start screen.
const (
	modeAuto        = iota // crop and write straight away
	modePreview            // open the preview, write on Save
	modePreviewOnly        // open the preview, never write
)

// Settings are the options panel's choices, remembered between runs.
type Settings struct {
	Mode        int
	MarginPt    float64
	Normalize   bool
	OCR         bool
	Straighten  bool
	AutoRotate  bool
	KeepHeaders bool
	SplitSheets bool // split two-page sheets (and pages stacked one above the other)
	SplitPages  int  // split each page: splitOff, splitColumns or splitRows
}

// Ways to split each page.
const (
	splitOff = iota
	splitColumns
	splitRows
)

func defaultSettings() Settings {
	return Settings{Mode: modeAuto, MarginPt: 6, Normalize: true, OCR: true, Straighten: true, AutoRotate: true, KeepHeaders: true, SplitSheets: true, SplitPages: splitOff}
}

func loadSettings(p fyne.Preferences) Settings {
	d := defaultSettings()
	return Settings{
		Mode:        p.IntWithFallback("mode", d.Mode),
		MarginPt:    p.FloatWithFallback("margin_pt", d.MarginPt),
		Normalize:   p.BoolWithFallback("normalize", d.Normalize),
		OCR:         p.BoolWithFallback("ocr", d.OCR),
		Straighten:  p.BoolWithFallback("straighten", d.Straighten),
		AutoRotate:  p.BoolWithFallback("auto_rotate", d.AutoRotate),
		SplitSheets: p.BoolWithFallback("split_sheets", d.SplitSheets),
		SplitPages:  p.IntWithFallback("split_pages", d.SplitPages),
		KeepHeaders: p.BoolWithFallback("keep_headers", d.KeepHeaders),
	}
}

func (s Settings) save(p fyne.Preferences) {
	p.SetInt("mode", s.Mode)
	p.SetFloat("margin_pt", s.MarginPt)
	p.SetBool("normalize", s.Normalize)
	p.SetBool("ocr", s.OCR)
	p.SetBool("straighten", s.Straighten)
	p.SetBool("auto_rotate", s.AutoRotate)
	p.SetBool("split_sheets", s.SplitSheets)
	p.SetInt("split_pages", s.SplitPages)
	p.SetBool("keep_headers", s.KeepHeaders)
}

// options turns the settings into what the processing core takes.
func (s Settings) options() (app.Options, app.Extras) {
	o := app.Options{Detector: "auto", Margin: app.Margin{Value: s.MarginPt}}
	o.Detect.KeepHeaders = s.KeepHeaders
	o.Detect.AutoRotate = s.AutoRotate
	o.Detect.NoAutoSplit = !s.SplitSheets
	o.Detect.SplitColumns = s.SplitPages == splitColumns
	o.Detect.SplitRows = s.SplitPages == splitRows
	return o, app.Extras{Straighten: s.Straighten, Normalize: s.Normalize, OCR: s.OCR, OCRLang: "eng"}
}
