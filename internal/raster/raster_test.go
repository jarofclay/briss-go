package raster

import (
	"image"
	"testing"
)

func TestRotateQuarterTurnMovesRightToTop(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 101, 101))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	g.Pix[50*g.Stride+90] = 0 // a dark pixel right of the centre
	r := Rotate(g, 90, 50.5, 50.5, 255)
	// counterclockwise on screen: right of the centre goes to above it
	if v := r.Pix[10*r.Stride+50]; v > 10 {
		t.Fatalf("pixel did not move above the centre: %d", v)
	}
	if v := r.Pix[50*r.Stride+90]; v < 250 {
		t.Fatalf("old position still dark: %d", v)
	}
}

func TestCrop(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 10, 10))
	g.Pix[3*g.Stride+4] = 7
	c := Crop(g, image.Rect(4, 3, 8, 9))
	if c.Bounds().Dx() != 4 || c.Bounds().Dy() != 6 || c.Pix[0] != 7 {
		t.Fatalf("crop: %v %d", c.Bounds(), c.Pix[0])
	}
}
