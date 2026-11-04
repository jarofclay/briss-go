package main

import (
	"image"
	"math"
)

// Port of the Java ImageFinderUtil / CropFinder auto-crop (tier T0).

const (
	ratioLookAheadSatisfy = 0.85
	lookAheadPixelNr      = 30
	sdCalcSizeNr          = 5
	sdThreshold           = 0.2
)

// legacyAutoCrop returns briss ratios {l,b,r,t} for a gray overlay.
func legacyAutoCrop(img *image.Gray) Ratios {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	sdX := sdOfDerivation(sumAxis(img, true))
	sdY := sdOfDerivation(sumAxis(img, false))
	left := findPosition(sdX, true)
	top := findPosition(sdY, true)
	right := findPosition(sdX, false)
	bottom := findPosition(sdY, false)
	return Ratios{
		L: float64(left) / float64(w),
		B: float64(h-bottom) / float64(h),
		R: float64(w-right) / float64(w),
		T: float64(top) / float64(h),
	}
}

func sumAxis(img *image.Gray, columns bool) []float64 {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if columns {
		v := make([]float64, w)
		for x := 0; x < w; x++ {
			var s float64
			for y := 0; y < h; y++ {
				s += float64(img.Pix[y*img.Stride+x])
			}
			v[x] = s / float64(h)
		}
		return v
	}
	v := make([]float64, h)
	for y := 0; y < h; y++ {
		var s float64
		row := img.Pix[y*img.Stride : y*img.Stride+w]
		for _, p := range row {
			s += float64(p)
		}
		v[y] = s / float64(w)
	}
	return v
}

func sdOfDerivation(sum []float64) []float64 {
	d := make([]float64, len(sum)-1)
	for i := range d {
		d[i] = sum[i+1] - sum[i]
	}
	sds := make([]float64, len(d))
	for i := range d {
		var tmp [sdCalcSizeNr]float64
		for j := 0; j < sdCalcSizeNr; j++ {
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
