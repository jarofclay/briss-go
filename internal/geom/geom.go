// Package geom holds the crop-rectangle model shared by every part of briss.
//
// A crop rectangle is stored as Ratios: the fraction of the page width or height
// cut away from the left, bottom, right and top edges, measured on the page as it
// is displayed (after /Rotate). This is the format the Java Briss used everywhere
// (float[4]{left, bottom, right, top}), so crop definitions stay interchangeable.
package geom

import "math"

// Ratios are briss crop ratios: fractions cut from the left, bottom, right and
// top edges of the displayed page. {0,0,0,0} keeps the whole page.
type Ratios struct {
	L, B, R, T float64
}

// Rect is a rectangle in PDF user space (points, origin bottom-left).
type Rect struct {
	LLX, LLY, URX, URY float64
}

// Width returns the rectangle width in points.
func (r Rect) Width() float64 { return r.URX - r.LLX }

// Height returns the rectangle height in points.
func (r Rect) Height() float64 { return r.URY - r.LLY }

// Area returns the rectangle area in square points.
func (r Rect) Area() float64 { return r.Width() * r.Height() }

// Valid reports whether the ratios describe a non-empty rectangle inside the page.
func (r Ratios) Valid() bool {
	in := func(v float64) bool { return v >= 0 && v <= 1 && !math.IsNaN(v) }
	return in(r.L) && in(r.B) && in(r.R) && in(r.T) && r.L+r.R < 1 && r.B+r.T < 1
}

// Clamp limits every ratio to [0,1] and keeps at least minSize of the page in
// each direction, so a rectangle dragged past the page edge stays usable.
func (r Ratios) Clamp(minSize float64) Ratios {
	c := func(v float64) float64 { return math.Max(0, math.Min(1, v)) }
	r = Ratios{c(r.L), c(r.B), c(r.R), c(r.T)}
	if r.L+r.R > 1-minSize {
		excess := (r.L + r.R) - (1 - minSize)
		r.L -= excess / 2
		r.R -= excess / 2
	}
	if r.B+r.T > 1-minSize {
		excess := (r.B + r.T) - (1 - minSize)
		r.B -= excess / 2
		r.T -= excess / 2
	}
	return Ratios{c(r.L), c(r.B), c(r.R), c(r.T)}
}

// IoU returns the intersection over union of two rectangles given as ratios.
func IoU(a, b Ratios) float64 {
	ax0, ay0, ax1, ay1 := a.L, a.T, 1-a.R, 1-a.B
	bx0, by0, bx1, by1 := b.L, b.T, 1-b.R, 1-b.B
	iw := math.Min(ax1, bx1) - math.Max(ax0, bx0)
	ih := math.Min(ay1, by1) - math.Max(ay0, by0)
	if iw <= 0 || ih <= 0 {
		return 0
	}
	inter := iw * ih
	union := (ax1-ax0)*(ay1-ay0) + (bx1-bx0)*(by1-by0) - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

// FromPixels converts a rectangle on a w x h image (top-left origin, as drawn in
// the GUI) to ratios, clipping it to the image like MergedPanel.getCutRatiosForPdf.
func FromPixels(x, y, rw, rh float64, w, h int) Ratios {
	x0, y0 := math.Max(0, x), math.Max(0, y)
	x1, y1 := math.Min(float64(w), x+rw), math.Min(float64(h), y+rh)
	fw, fh := float64(w), float64(h)
	return Ratios{L: x0 / fw, B: 1 - y1/fh, R: 1 - x1/fw, T: y0 / fh}
}

// ToPixels converts ratios to a rectangle on a w x h image (top-left origin).
func (r Ratios) ToPixels(w, h int) (x, y, rw, rh float64) {
	fw, fh := float64(w), float64(h)
	return r.L * fw, r.T * fh, (1 - r.L - r.R) * fw, (1 - r.B - r.T) * fh
}

// NormalizeRotation maps any /Rotate value to 0, 90, 180 or 270. Values that are
// not multiples of 90 are invalid in PDF and are treated as 0.
func NormalizeRotation(rotation int) int {
	r := ((rotation % 360) + 360) % 360
	if r%90 != 0 {
		return 0
	}
	return r
}

// Unrotate converts ratios measured on the displayed page to ratios on the
// unrotated page, for a page whose /Rotate is rotation. It ports
// RectangleHandler.rotateRatios: the ratios are shifted once per quarter turn
// needed to come back to 0 degrees.
func (r Ratios) Unrotate(rotation int) Ratios {
	rot := NormalizeRotation(rotation)
	for rot > 0 && rot < 360 {
		r = Ratios{L: r.B, B: r.R, R: r.T, T: r.L}
		rot += 90
	}
	return r
}

// ScaledBox ports RectangleHandler.calculateScaledRectangle: it takes the
// smaller (by area) of the media box and the crop box as the reference, rotates
// the ratios back to the unrotated page, and returns the crop rectangle in PDF
// user space. crop may be nil when the page has no /CropBox.
func ScaledBox(media Rect, crop *Rect, r Ratios, rotation int) Rect {
	ref := media
	if crop != nil && crop.Area() < media.Area() {
		ref = *crop
	}
	u := r.Unrotate(rotation)
	w, h := ref.Width(), ref.Height()
	return Rect{
		LLX: ref.LLX + w*u.L,
		LLY: ref.LLY + h*u.B,
		URX: ref.LLX + w*(1-u.R),
		URY: ref.LLY + h*(1-u.T),
	}
}
