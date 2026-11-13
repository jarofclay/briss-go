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

func TestRotateQuarter(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 4, 2))
	g.Pix[0] = 9 // top-left
	r := RotateQuarter(g, 90)
	if r.Bounds().Dx() != 2 || r.Bounds().Dy() != 4 || r.Pix[1] != 9 {
		t.Fatalf("90: top-left must go to top-right: %v %v", r.Bounds(), r.Pix)
	}
	if r := RotateQuarter(g, 180); r.Pix[len(r.Pix)-1] != 9 {
		t.Fatal("180: top-left must go to bottom-right")
	}
	if r := RotateQuarter(g, 270); r.Pix[(4-1)*r.Stride] != 9 {
		t.Fatal("270: top-left must go to bottom-left")
	}
	if back := RotateQuarter(RotateQuarter(g, 90), 270); back.Pix[0] != 9 || back.Bounds() != g.Bounds() {
		t.Fatal("90 then 270 must be the identity")
	}
}

func TestResizeAverages(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 4, 2))
	copy(g.Pix, []uint8{0, 200, 100, 100, 0, 200, 100, 100})
	r := Resize(g, 2, 1)
	if r.Bounds().Dx() != 2 || r.Bounds().Dy() != 1 || r.Pix[0] != 100 || r.Pix[1] != 100 {
		t.Fatalf("got %v %v", r.Bounds(), r.Pix)
	}
}

func TestPad(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 2, 2)) // black
	g := Pad(src, 4, 3, 1, 1, 255)
	if g.Bounds().Dx() != 4 || g.Bounds().Dy() != 3 {
		t.Fatalf("size %v", g.Bounds())
	}
	want := []uint8{
		255, 255, 255, 255,
		255, 0, 0, 255,
		255, 0, 0, 255,
	}
	for i, v := range want {
		if g.Pix[i] != v {
			t.Fatalf("pixel %d is %d, want %d (%v)", i, g.Pix[i], v, g.Pix)
		}
	}
	if g := Pad(src, 2, 2, -1, 1, 9); g.Pix[0] != 9 || g.Pix[2] != 0 || g.Pix[3] != 9 {
		t.Fatalf("cut off wrongly: %v", g.Pix)
	}
}
