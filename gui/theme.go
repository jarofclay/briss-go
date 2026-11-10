package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Palette: near-black surfaces, soft white text, one blue accent.
var (
	colBackground = rgb(0x0b, 0x0b, 0x0d)
	colSurface    = rgb(0x15, 0x15, 0x19)
	colRaised     = rgb(0x1d, 0x1d, 0x22)
	colBorder     = rgb(0x2a, 0x2a, 0x31)
	colText       = rgb(0xec, 0xec, 0xef)
	colDim        = rgb(0x8a, 0x8a, 0x94)
	colFaint      = rgb(0x55, 0x55, 0x5e)
	colAccent     = rgb(0x4d, 0x8d, 0xff)
	colSuccess    = rgb(0x3d, 0xd6, 0x8c)
	colError      = rgb(0xff, 0x5c, 0x5c)
)

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{R: r, G: g, B: b, A: 0xff} }

func withAlpha(c color.NRGBA, a uint8) color.NRGBA { c.A = a; return c }

// darkTheme is always dark, whatever the system setting.
type darkTheme struct{}

func (darkTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return colBackground
	case theme.ColorNameButton, theme.ColorNameInputBackground, theme.ColorNameMenuBackground, theme.ColorNameHeaderBackground:
		return colRaised
	case theme.ColorNameOverlayBackground:
		return colSurface
	case theme.ColorNameDisabledButton:
		return colSurface
	case theme.ColorNameDisabled, theme.ColorNamePlaceHolder:
		return colFaint
	case theme.ColorNameForeground:
		return colText
	case theme.ColorNameForegroundOnPrimary, theme.ColorNameForegroundOnSuccess, theme.ColorNameForegroundOnError, theme.ColorNameForegroundOnWarning:
		return colBackground
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return colAccent
	case theme.ColorNameSelection:
		return withAlpha(colAccent, 0x40)
	case theme.ColorNameHover:
		return withAlpha(colText, 0x0f)
	case theme.ColorNamePressed:
		return withAlpha(colText, 0x1a)
	case theme.ColorNameInputBorder, theme.ColorNameSeparator:
		return colBorder
	case theme.ColorNameScrollBar:
		return withAlpha(colText, 0x30)
	case theme.ColorNameScrollBarBackground:
		return color.Transparent
	case theme.ColorNameShadow:
		return color.NRGBA{A: 0x80}
	case theme.ColorNameSuccess:
		return colSuccess
	case theme.ColorNameError:
		return colError
	case theme.ColorNameWarning:
		return rgb(0xff, 0xb0, 0x3d)
	}
	return theme.DefaultTheme().Color(n, theme.VariantDark)
}

func (darkTheme) Font(s fyne.TextStyle) fyne.Resource { return theme.DefaultTheme().Font(s) }

func (darkTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }

func (darkTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameText:
		return 13
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameInputRadius, theme.SizeNameButtonRadius, theme.SizeNameSelectionRadius:
		return 8
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameScrollBarSmall:
		return 3
	}
	return theme.DefaultTheme().Size(n)
}
