package ocr

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func TestParseTSV(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"1\t1\t0\t0\t0\t0\t0\t0\t800\t600\t-1\t\n" +
		"5\t1\t1\t1\t1\t1\t100\t50\t60\t20\t96.5\tHello\n" +
		"5\t1\t1\t1\t1\t2\t170\t50\t70\t20\t91\tworld,\n" +
		"5\t1\t1\t1\t1\t3\t250\t50\t10\t20\t95\t \n"
	w, err := ParseTSV([]byte(tsv))
	if err != nil {
		t.Fatal(err)
	}
	if len(w) != 2 || w[0].Text != "Hello" || w[0].X1 != 160 || w[0].Y1 != 70 || w[1].Text != "world," {
		t.Fatalf("got %+v", w)
	}
}

// Draws a line of text, scaled up, and reads it back. Skipped without Tesseract.
func TestRecognize(t *testing.T) {
	bin, err := Find()
	if err != nil {
		t.Skip(err)
	}
	small := image.NewGray(image.Rect(0, 0, 220, 30))
	draw.Draw(small, small.Bounds(), image.White, image.Point{}, draw.Src)
	d := font.Drawer{Dst: small, Src: image.NewUniform(color.Black), Face: basicfont.Face7x13, Dot: fixed.P(8, 20)}
	d.DrawString("briss reads scans")
	big := image.NewGray(image.Rect(0, 0, 220*4, 30*4))
	for y := 0; y < big.Bounds().Dy(); y++ {
		for x := 0; x < big.Bounds().Dx(); x++ {
			big.Pix[y*big.Stride+x] = small.Pix[(y/4)*small.Stride+x/4]
		}
	}
	words, err := Recognize(context.Background(), bin, big, "eng")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, w := range words {
		got = append(got, w.Text)
	}
	if s := strings.Join(got, " "); !strings.Contains(s, "reads") || !strings.Contains(s, "scans") {
		t.Fatalf("recognised %q", s)
	}
	if words[0].X0 < 20 || words[0].X0 > 60 {
		t.Fatalf("first word box %+v", words[0])
	}
}
