// Package legacy is detection tier T0: a bit-compatible port of the Java
// Briss auto-crop (ImageFinderUtil, CropFinder) and column/row split
// (SplitFinder). It keeps running headers and is fooled by scan noise; it
// exists so that the legacy command line behaves exactly as before and as the
// baseline every other tier is measured against.
package legacy

import (
	"context"
	"errors"
	"image"
	"math"

	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/geom"
)

// Name is the detector name used in crop definitions.
const Name = "legacy@1"

const (
	ratioLookAheadSatisfy = 0.85
	lookAheadPixelNr      = 30
	sdCalcSizeNr          = 5
	sdThreshold           = 0.2

	lookRatio       = 0.5
	maxDistRatio    = 0.1
	rowOverlapRatio = float32(0.01)
)

// Detector adapts the legacy algorithm to detect.Detector.
type Detector struct{}

// Name implements detect.Detector.
func (Detector) Name() string { return Name }

// Available implements detect.Detector; the legacy tier always runs.
func (Detector) Available(context.Context) error { return nil }

// Detect implements detect.Detector. An overlay without any detectable edge
// (a blank page) yields the whole page instead of Java's degenerate rectangle.
func (Detector) Detect(_ context.Context, req detect.Request) (detect.Candidate, error) {
	if req.Overlay == nil {
		return detect.Candidate{}, errors.New("legacy: overlay required")
	}
	auto := AutoCrop(req.Overlay)
	c := detect.Candidate{Source: Name, Confidence: 0.5}
	if !auto.Valid() {
		auto = geom.Ratios{}
		c.Confidence = 0
		c.Notes = "no content edges found; keeping the whole page"
	}
	rects := []geom.Ratios{auto}
	if req.Options.SplitColumns {
		var next []geom.Ratios
		for _, r := range rects {
			next = append(next, SplitColumn(req.Overlay, r)...)
		}
		rects = next
	}
	if req.Options.SplitRows {
		var next []geom.Ratios
		for _, r := range rects {
			next = append(next, SplitRow(req.Overlay, r)...)
		}
		rects = next
	}
	c.Rects = rects
	return c, nil
}

// Profile holds the mean gray value of every column and row of an overlay,
// the only input the legacy algorithm uses.
type Profile struct {
	Cols, Rows []float64
}

// ProfileOf computes the profile of an overlay like ImageFinderUtil.sumFrom2dTo1d.
func ProfileOf(img *image.Gray) Profile {
	return Profile{Cols: sumAxis(img, true), Rows: sumAxis(img, false)}
}

// ProfileFromSums builds a profile from integer column and row sums (as stored
// in the Java golden data); means are sum/height and sum/width in float64,
// which is exactly what Java's double accumulation produces.
func ProfileFromSums(colSums, rowSums []int64) Profile {
	w, h := len(colSums), len(rowSums)
	p := Profile{Cols: make([]float64, w), Rows: make([]float64, h)}
	for i, v := range colSums {
		p.Cols[i] = float64(v) / float64(h)
	}
	for i, v := range rowSums {
		p.Rows[i] = float64(v) / float64(w)
	}
	return p
}

// AutoCrop ports CropFinder.getAutoCropFloats.
func AutoCrop(img *image.Gray) geom.Ratios { return AutoCropProfile(ProfileOf(img)) }

// AutoCropProfile runs the auto-crop on a profile.
func AutoCropProfile(p Profile) geom.Ratios {
	w, h := len(p.Cols), len(p.Rows)
	sdX := sdOfDerivation(p.Cols)
	sdY := sdOfDerivation(p.Rows)
	left := findPosition(sdX, true)
	top := findPosition(sdY, true)
	right := findPosition(sdX, false)
	bottom := findPosition(sdY, false)
	// Java divides in float32
	return geom.Ratios{
		L: float64(float32(left) / float32(w)),
		B: float64(float32(h-bottom) / float32(h)),
		R: float64(float32(w-right) / float32(w)),
		T: float64(float32(top) / float32(h)),
	}
}

// SplitColumn ports SplitFinder.splitColumn: the split position is searched in
// the middle 10% of the whole overlay (not of the rectangle, a Java TODO) and
// applied proportionally inside the rectangle, in float32 like Java.
func SplitColumn(img *image.Gray, r geom.Ratios) []geom.Ratios {
	return SplitColumnProfile(ProfileOf(img), r)
}

// SplitColumnProfile is SplitColumn on a profile.
func SplitColumnProfile(p Profile, r geom.Ratios) []geom.Ratios {
	ratio := splitRatio(sdOfDerivation(p.Cols), len(p.Cols))
	l, b, rr, t := float32(r.L), float32(r.B), float32(r.R), float32(r.T)
	width := 1 - rr - l
	divider := l + ratio*width
	return []geom.Ratios{
		f32(l, b, 1-divider, t),
		f32(divider, b, rr, t),
	}
}

// SplitRow ports SplitFinder.splitRow, the top part first, both parts
// overlapping by 1% of the page. Java computes the search range and the ratio
// with the overlay width also for rows; that quirk is kept so the legacy
// --split-row flag behaves as before (an improved split belongs to tier T2).
func SplitRow(img *image.Gray, r geom.Ratios) []geom.Ratios {
	return SplitRowProfile(ProfileOf(img), r)
}

// SplitRowProfile is SplitRow on a profile.
func SplitRowProfile(p Profile, r geom.Ratios) []geom.Ratios {
	ratio := splitRatio(sdOfDerivation(p.Rows), len(p.Cols))
	l, b, rr, t := float32(r.L), float32(r.B), float32(r.R), float32(r.T)
	height := 1 - b - t
	divider := t + ratio*height
	return []geom.Ratios{
		f32(l, 1-divider-rowOverlapRatio, rr, t),
		f32(l, b, rr, divider-rowOverlapRatio),
	}
}

func f32(l, b, r, t float32) geom.Ratios {
	return geom.Ratios{L: float64(l), B: float64(b), R: float64(r), T: float64(t)}
}

// splitRatio ports SplitFinder.getSplitRatio; width is the overlay width for
// both axes, as in Java.
func splitRatio(sds []float64, width int) float32 {
	start := int(math.Floor(float64(width) * (lookRatio - maxDistRatio/2)))
	end := int(math.Ceil(float64(width) * (lookRatio + maxDistRatio/2)))
	end = min(end, len(sds)) // Java would throw here on very wide pages
	best, idx := math.MaxFloat64, -1
	for i := start; i < end; i++ {
		if sds[i] < best {
			best, idx = sds[i], i
		}
	}
	if idx < 0 {
		return 0.5
	}
	return float32(idx) / float32(width)
}

func sumAxis(img *image.Gray, columns bool) []float64 {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if columns {
		v := make([]float64, w)
		for y := 0; y < h; y++ {
			row := img.Pix[y*img.Stride : y*img.Stride+w]
			for x, p := range row {
				v[x] += float64(p)
			}
		}
		for x := range v {
			v[x] /= float64(h)
		}
		return v
	}
	v := make([]float64, h)
	for y := 0; y < h; y++ {
		var s float64
		for _, p := range img.Pix[y*img.Stride : y*img.Stride+w] {
			s += float64(p)
		}
		v[y] = s / float64(w)
	}
	return v
}

func sdOfDerivation(sum []float64) []float64 {
	if len(sum) < 2 {
		return nil
	}
	d := make([]float64, len(sum)-1)
	for i := range d {
		d[i] = sum[i+1] - sum[i]
	}
	sds := make([]float64, len(d))
	var tmp [sdCalcSizeNr]float64
	for i := range d {
		for j := 0; j < sdCalcSizeNr; j++ {
			tmp[j] = 0
			if i+j < len(d) {
				tmp[j] = d[i+j]
			}
		}
		sds[i] = sd(tmp[:])
	}
	return sds
}

func sd(v []float64) float64 {
	var mean float64
	for _, x := range v {
		mean += x
	}
	mean /= float64(len(v))
	var s float64
	for _, x := range v {
		s += (x - mean) * (x - mean)
	}
	return math.Sqrt(s / float64(len(v)))
}

func findPosition(sds []float64, fromStart bool) int {
	if fromStart {
		for i := 0; i < len(sds)-lookAheadPixelNr; i++ {
			if float64(diffCounter(sds, i, i+lookAheadPixelNr)) > ratioLookAheadSatisfy*lookAheadPixelNr {
				return i
			}
		}
		return 0
	}
	for i := len(sds) - 1; i >= lookAheadPixelNr; i-- {
		if float64(diffCounter(sds, i-lookAheadPixelNr, i)) > ratioLookAheadSatisfy*lookAheadPixelNr {
			return i
		}
	}
	return 0
}

func diffCounter(v []float64, start, end int) int {
	n := 0
	for i := start; i < end; i++ {
		if v[i] > sdThreshold {
			n++
		}
	}
	return n
}
